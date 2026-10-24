package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/MurabitoB/gh-9s/internal/markdown"
	"github.com/MurabitoB/gh-9s/internal/ui"
)

// detailDoc is a detail screen laid out as lines, plus where its image markers are.
type detailDoc struct {
	lines  []string
	images []markdown.Image
}

func (d *detailDoc) add(lines ...string) { d.lines = append(d.lines, lines...) }

// addMarkdown appends body rendered as markdown, every line prefixed with indent.
func (d *detailDoc) addMarkdown(body string, width int, indent string) {
	md := markdown.Render(body, max(width-len([]rune(indent)), 1))
	for _, im := range md.Images {
		im.Line += len(d.lines)
		d.images = append(d.images, im)
	}
	for _, l := range md.Lines {
		if l == "" {
			d.lines = append(d.lines, "")
		} else {
			d.lines = append(d.lines, indent+l)
		}
	}
}

func (d *detailDoc) addDoc(o detailDoc) {
	for _, im := range o.images {
		im.Line += len(d.lines)
		d.images = append(d.images, im)
	}
	d.lines = append(d.lines, o.lines...)
}

// imageKeys handles the keys that work on image markers. sel is 1+index of the selected marker (0
// none). It returns the new selection and scroll, whether the key was used, and the URL when the
// key asks to show or open one.
type imageAction int

const (
	imgNone imageAction = iota
	imgMoved
	imgShow
	imgOpen
)

func imageKeys(d detailDoc, sel, scroll, h int, key string) (newSel, newScroll int, act imageAction) {
	n := len(d.images)
	switch key {
	case "tab", "shift+tab":
		if n == 0 {
			return sel, scroll, imgNone
		}
		if key == "tab" {
			sel = sel%n + 1
		} else {
			sel = (sel+n-2)%n + 1
			if sel < 1 {
				sel = n
			}
		}
		line := d.images[sel-1].Line
		maxScroll := max(0, len(d.lines)-h)
		if line < scroll || line >= scroll+h {
			scroll = min(max(line-h/2, 0), maxScroll)
		}
		return sel, scroll, imgMoved
	case "enter":
		if sel > 0 && sel <= n {
			return sel, scroll, imgShow
		}
	case "o":
		if sel > 0 && sel <= n {
			return sel, scroll, imgOpen
		}
	}
	return sel, scroll, imgNone
}

// paintDoc writes the visible part of d, with the selected image marker highlighted.
func paintDoc(b *strings.Builder, d detailDoc, sel, scroll, h int) {
	n := 0
	for i := min(scroll, max(0, len(d.lines)-1)); i < len(d.lines) && n < h; i++ {
		l := d.lines[i]
		if sel > 0 && sel <= len(d.images) && d.images[sel-1].Line == i {
			l = ui.Sel.Render(ansi.Strip(l))
		}
		b.WriteString(" " + l + "\n")
		n++
	}
	for ; n < h; n++ {
		b.WriteString("\n")
	}
}
