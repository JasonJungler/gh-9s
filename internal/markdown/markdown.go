// Package markdown turns a PR or issue description into styled terminal lines. The text is cleaned
// of escape sequences first; the only escape sequences in the output are this package's own styling.
package markdown

import (
	"crypto/sha256"
	"regexp"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"

	"github.com/MurabitoB/gh-9s/internal/safe"
)

// Image is an image reference found in the text; Line is its marker's index in Doc.Lines.
type Image struct {
	Line     int
	Alt, URL string
}

// Doc is the rendered text: Lines never exceed the requested width.
type Doc struct {
	Lines  []string
	Images []Image
}

const (
	maxInput = 1 << 20 // bytes of markdown that are rendered
	maxDepth = 32      // nesting beyond this is shown flat
)

var (
	parser = goldmark.New(goldmark.WithExtensions(extension.Table, extension.Strikethrough, extension.TaskList)).Parser()

	styH      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	styBold   = lipgloss.NewStyle().Bold(true)
	styItalic = lipgloss.NewStyle().Italic(true)
	styStrike = lipgloss.NewStyle().Strikethrough(true)
	styCode   = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	styBlock  = lipgloss.NewStyle().Background(lipgloss.Color("236"))
	styLink   = lipgloss.NewStyle().Underline(true)
	styDim    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styImage  = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))

	imgTag  = regexp.MustCompile(`(?is)<img\b[^>]*>`)
	srcAttr = regexp.MustCompile(`(?is)\bsrc\s*=\s*["']([^"']*)["']`)
	altAttr = regexp.MustCompile(`(?is)\balt\s*=\s*["']([^"']*)["']`)
	anyTag  = regexp.MustCompile(`(?s)<!--.*?-->|</?[A-Za-z][^>]*>`)
)

var (
	parses     int // how many times Render really parsed; for the tests
	cacheMu    sync.Mutex
	cache      = map[[32]byte]Doc{}
	blockStart = regexp.MustCompile(`^ {0,3}(?:>[ ]?|[-*+] |[0-9]{1,9}[.)] )`)
)

const maxCache = 64

// limitNesting keeps at most maxDepth block markers (">", "-", "1.") at the start of a line and
// at most 64 columns of indent: deeper nesting costs the parser quadratic time and is shown flat.
func limitNesting(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		trimmed := strings.TrimLeft(l, " ")
		if indent := len(l) - len(trimmed); indent > 64 {
			l = strings.Repeat(" ", 64) + trimmed
		}
		rest, kept, n := l, strings.Builder{}, 0
		for {
			m := blockStart.FindString(rest)
			if m == "" {
				break
			}
			rest = rest[len(m):]
			if n++; n <= maxDepth {
				kept.WriteString(m)
			}
		}
		if n > maxDepth {
			lines[i] = kept.String() + rest
		} else {
			lines[i] = l
		}
	}
	return strings.Join(lines, "\n")
}

// line is one output line; img is 1+index into the collected images, or 0.
type line struct {
	s   string
	img int
}

type renderer struct {
	src    []byte
	images []Image
}

// Render lays src out in lines of at most width columns. Results are cached; treat them as read-only.
func Render(src string, width int) Doc {
	width = max(width, 1)
	if len(src) > maxInput {
		src = src[:maxInput]
	}
	key := sha256.Sum256([]byte(string(rune(width)) + "\x00" + src))
	cacheMu.Lock()
	if d, ok := cache[key]; ok {
		cacheMu.Unlock()
		return d
	}
	cacheMu.Unlock()
	doc := render(src, width)
	cacheMu.Lock()
	if len(cache) >= maxCache {
		cache = map[[32]byte]Doc{}
	}
	cache[key] = doc
	cacheMu.Unlock()
	return doc
}

func render(src string, width int) Doc {
	parses++
	b := []byte(limitNesting(strings.ReplaceAll(safe.Text(src), "\t", "    ")))
	root := parser.Parse(text.NewReader(b))
	r := &renderer{src: b}
	lines := r.blocks(root, width, 0)
	var doc Doc
	doc.Images = r.images
	for _, l := range lines {
		s := l.s
		if ansi.StringWidth(s) > width {
			s = ansi.Truncate(s, width, "…")
		}
		if l.img > 0 {
			doc.Images[l.img-1].Line = len(doc.Lines)
		}
		doc.Lines = append(doc.Lines, s)
	}
	return doc
}

// blocks renders the children of n one after another, a blank line between blocks.
func (r *renderer) blocks(n ast.Node, w, depth int) []line {
	var out []line
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		ls := r.block(c, w, depth)
		if len(ls) == 0 {
			continue
		}
		if len(out) > 0 && !tightPair(c) && !inTightItem(c) {
			out = append(out, line{})
		}
		out = append(out, ls...)
	}
	return out
}

// tightPair: items of a tight list follow each other without a blank line.
func tightPair(c ast.Node) bool {
	if c.Kind() != ast.KindListItem {
		return false
	}
	l, ok := c.Parent().(*ast.List)
	return ok && l.IsTight
}

// inTightItem: blocks inside an item of a tight list follow each other without a blank line.
func inTightItem(c ast.Node) bool {
	it := c.Parent()
	if it == nil || it.Kind() != ast.KindListItem {
		return false
	}
	l, ok := it.Parent().(*ast.List)
	return ok && l.IsTight
}

func (r *renderer) block(n ast.Node, w, depth int) []line {
	if depth > maxDepth {
		return plainLines(r.flat(n), w)
	}
	switch n := n.(type) {
	case *ast.Heading:
		return r.inlineBlock(n, w, &styH)
	case *ast.Paragraph, *ast.TextBlock:
		return r.inlineBlock(n, w, nil)
	case *ast.FencedCodeBlock, *ast.CodeBlock:
		var out []line
		for i := 0; i < n.Lines().Len(); i++ {
			seg := n.Lines().At(i)
			l := strings.TrimRight(string(seg.Value(r.src)), "\n")
			out = append(out, line{s: styBlock.Render("  " + ansi.Truncate(strings.ReplaceAll(l, "\t", "    "), max(w-2, 1), "…"))})
		}
		return out
	case *ast.Blockquote:
		if w < 12 { // no room left for another level: show the rest as it is
			return plainLines(r.flat(n), w)
		}
		inner := r.blocks(n, max(w-2, 1), depth+1)
		for i := range inner {
			inner[i].s = styDim.Render("│ ") + inner[i].s
		}
		return inner
	case *ast.List:
		if w < 12 {
			return plainLines(r.flat(n), w)
		}
		return r.list(n, w, depth)
	case *ast.ListItem:
		return r.blocks(n, w, depth+1)
	case *ast.ThematicBreak:
		return []line{{s: styDim.Render(strings.Repeat("─", min(w, 40)))}}
	case *ast.HTMLBlock:
		var raw strings.Builder
		for i := 0; i < n.Lines().Len(); i++ {
			seg := n.Lines().At(i)
			raw.Write(seg.Value(r.src))
		}
		return r.htmlText(raw.String(), w)
	case *extast.Table:
		return r.table(n, w)
	}
	// unknown block: show its text rather than lose it
	if n.HasChildren() {
		return r.blocks(n, w, depth+1)
	}
	return nil
}

func (r *renderer) list(l *ast.List, w, depth int) []line {
	var out []line
	num := l.Start
	for it := l.FirstChild(); it != nil; it = it.NextSibling() {
		prefix := "• "
		if l.IsOrdered() {
			prefix = itoa(num) + ". "
			num++
		}
		body := r.blocks(it, max(w-ansi.StringWidth(prefix), 1), depth+1)
		if len(body) == 0 {
			body = []line{{}}
		}
		pad := strings.Repeat(" ", ansi.StringWidth(prefix))
		for i, b := range body {
			if i == 0 {
				b.s = prefix + b.s
			} else if b.s != "" {
				b.s = pad + b.s
			}
			body[i] = b
		}
		if len(out) > 0 && !l.IsTight {
			out = append(out, line{})
		}
		out = append(out, body...)
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var d []byte
	for ; n > 0; n /= 10 {
		d = append([]byte{byte('0' + n%10)}, d...)
	}
	if neg {
		d = append([]byte{'-'}, d...)
	}
	return string(d)
}

// seg is a run of inline text or an image.
type seg struct {
	s   string
	img *Image
}

// inlineBlock renders a paragraph or heading: wrapped text, with each image on a line of its own.
func (r *renderer) inlineBlock(n ast.Node, w int, sty *lipgloss.Style) []line {
	var segs []seg
	r.inline(n, &segs, nil)
	var out []line
	var cur strings.Builder
	flush := func() {
		s := strings.TrimSpace(cur.String())
		cur.Reset()
		if s == "" {
			return
		}
		for _, l := range strings.Split(ansi.Wrap(s, w, ""), "\n") {
			if sty != nil {
				l = sty.Render(l)
			}
			out = append(out, line{s: l})
		}
	}
	for _, sg := range segs {
		if sg.img == nil {
			cur.WriteString(sg.s)
			continue
		}
		flush()
		out = append(out, r.imageLine(*sg.img))
	}
	flush()
	return out
}

func (r *renderer) imageLine(im Image) line {
	r.images = append(r.images, im)
	label := "[image]"
	if im.Alt != "" {
		label = "[image: " + im.Alt + "]"
	}
	return line{s: styImage.Render(label), img: len(r.images)}
}

// inline appends the inline content of n to segs; wrap, when set, styles the plain text.
func (r *renderer) inline(n ast.Node, segs *[]seg, wrap *lipgloss.Style) {
	add := func(s string) {
		if wrap != nil && s != "" {
			s = wrap.Render(s)
		}
		*segs = append(*segs, seg{s: s})
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch c := c.(type) {
		case *ast.Text:
			add(string(c.Segment.Value(r.src)))
			if c.HardLineBreak() || c.SoftLineBreak() { // GitHub shows a single newline as a line break
				add("\n")
			}
		case *ast.String:
			add(string(c.Value))
		case *ast.CodeSpan:
			add(styCode.Render(r.flat(c)))
		case *ast.Emphasis:
			sty := styItalic
			if c.Level >= 2 {
				sty = styBold
			}
			r.inlineStyled(c, segs, sty)
		case *extast.Strikethrough:
			r.inlineStyled(c, segs, styStrike)
		case *ast.Link:
			r.inlineStyled(c, segs, styLink)
		case *ast.AutoLink:
			add(string(c.URL(r.src)))
		case *ast.Image:
			*segs = append(*segs, seg{img: &Image{Alt: strings.TrimSpace(r.flat(c)), URL: string(c.Destination)}})
		case *extast.TaskCheckBox:
			if c.IsChecked {
				add("☑ ")
			} else {
				add("☐ ")
			}
		case *ast.RawHTML:
			var raw strings.Builder
			for i := 0; i < c.Segments.Len(); i++ {
				seg := c.Segments.At(i)
				raw.Write(seg.Value(r.src))
			}
			if im, ok := imageFromTag(raw.String()); ok {
				*segs = append(*segs, seg{img: &im})
			}
		default:
			r.inline(c, segs, wrap)
		}
	}
}

// inlineStyled renders c's children and applies sty to each text run (images stay as they are).
func (r *renderer) inlineStyled(c ast.Node, segs *[]seg, sty lipgloss.Style) {
	var inner []seg
	r.inline(c, &inner, nil)
	for _, s := range inner {
		if s.img == nil && strings.TrimSpace(s.s) != "" {
			s.s = sty.Render(s.s)
		}
		*segs = append(*segs, s)
	}
}

func imageFromTag(tag string) (Image, bool) {
	if !imgTag.MatchString(tag) {
		return Image{}, false
	}
	m := srcAttr.FindStringSubmatch(tag)
	if m == nil {
		return Image{}, false
	}
	im := Image{URL: m[1]}
	if a := altAttr.FindStringSubmatch(tag); a != nil {
		im.Alt = strings.TrimSpace(a[1])
	}
	return im, true
}

// htmlText shows an HTML block as text: comments and tags dropped, <img> kept as an image.
func (r *renderer) htmlText(raw string, w int) []line {
	var out []line
	for _, tag := range imgTag.FindAllString(raw, -1) {
		if im, ok := imageFromTag(tag); ok {
			out = append(out, r.imageLine(im))
		}
	}
	rest := strings.TrimSpace(anyTag.ReplaceAllString(raw, ""))
	if rest != "" {
		out = append(plainLines(rest, w), out...)
	}
	return out
}

func plainLines(s string, w int) []line {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var out []line
	for _, l := range strings.Split(ansi.Wrap(s, w, ""), "\n") {
		out = append(out, line{s: l})
	}
	return out
}

// flat is the plain text under n.
func (r *renderer) flat(n ast.Node) string {
	var b strings.Builder
	var walk func(ast.Node, int)
	walk = func(c ast.Node, d int) {
		if d > 200 {
			return
		}
		switch c := c.(type) {
		case *ast.Text:
			b.Write(c.Segment.Value(r.src))
			if c.SoftLineBreak() || c.HardLineBreak() {
				b.WriteByte(' ')
			}
		case *ast.String:
			b.Write(c.Value)
		}
		for k := c.FirstChild(); k != nil; k = k.NextSibling() {
			walk(k, d+1)
		}
		if c.Type() == ast.TypeBlock && c.Kind() != ast.KindDocument {
			if c.Lines() != nil {
				for i := 0; i < c.Lines().Len(); i++ {
					seg := c.Lines().At(i)
					if !c.HasChildren() {
						b.Write(seg.Value(r.src))
					}
				}
			}
		}
	}
	walk(n, 0)
	return b.String()
}

func (r *renderer) table(t *extast.Table, w int) []line {
	var rows [][]string
	header := 0
	for row := t.FirstChild(); row != nil; row = row.NextSibling() {
		var cells []string
		for c := row.FirstChild(); c != nil; c = c.NextSibling() {
			var segs []seg
			r.inline(c, &segs, nil)
			var sb strings.Builder
			for _, s := range segs {
				if s.img != nil {
					sb.WriteString("[image]")
				} else {
					sb.WriteString(s.s)
				}
			}
			cells = append(cells, strings.ReplaceAll(strings.TrimSpace(sb.String()), "\n", " "))
		}
		if row.Kind() == extast.KindTableHeader {
			header = len(rows)
		}
		rows = append(rows, cells)
	}
	cols := 0
	for _, row := range rows {
		cols = max(cols, len(row))
	}
	if cols == 0 {
		return nil
	}
	widths := make([]int, cols)
	for _, row := range rows {
		for i, c := range row {
			widths[i] = max(widths[i], ansi.StringWidth(c))
		}
	}
	const sep = " │ "
	total := func() int {
		t := (cols - 1) * 3
		for _, x := range widths {
			t += x
		}
		return t
	}
	for total() > w {
		big, at := 0, -1
		for i, x := range widths {
			if x > big {
				big, at = x, i
			}
		}
		if at < 0 || widths[at] <= 3 {
			break
		}
		widths[at]--
	}
	var out []line
	for ri, row := range rows {
		parts := make([]string, cols)
		for i := range parts {
			c := ""
			if i < len(row) {
				c = row[i]
			}
			if ansi.StringWidth(c) > widths[i] {
				c = ansi.Truncate(c, widths[i], "…")
			}
			c += strings.Repeat(" ", max(widths[i]-ansi.StringWidth(c), 0))
			if ri == header {
				c = styBold.Render(c)
			}
			parts[i] = c
		}
		out = append(out, line{s: strings.TrimRight(strings.Join(parts, sep), " ")})
		if ri == header {
			rule := make([]string, cols)
			for i := range rule {
				rule[i] = strings.Repeat("─", widths[i])
			}
			out = append(out, line{s: styDim.Render(strings.Join(rule, "─┼─"))})
		}
	}
	return out
}
