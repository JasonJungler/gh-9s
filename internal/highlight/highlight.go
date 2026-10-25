// Package highlight colours the code in a diff by language, with chroma. A hunk's old side
// (context and deleted lines) and new side (context and added lines) are each tokenised as one
// piece of text, so strings and comments that span lines inside a hunk come out right. Something
// that starts before the hunk can still be coloured wrongly: that is the price of not fetching
// whole files.
package highlight

import (
	"crypto/sha256"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"

	"github.com/MurabitoB/gh-9s/internal/diff"
)

const (
	maxLineLen  = 400 // some lexers are far slower than linear on long lines
	maxFileLine = 20000
)

// Seg is a stretch of one line in one colour. FG is "#rrggbb", or empty for the terminal's own.
type Seg struct {
	Text         string
	FG           string
	Bold, Italic bool
}

// File holds the highlighted segments of a file's diff lines, by index in diff.File.Lines.
type File struct{ left, right map[int][]Seg }

// Left is a line as it reads in the old file (context and deleted lines); nil when not coloured.
func (f *File) Left(i int) []Seg {
	if f == nil {
		return nil
	}
	return f.left[i]
}

// Right is a line as it reads in the new file (context and added lines); nil when not coloured.
func (f *File) Right(i int) []Seg {
	if f == nil {
		return nil
	}
	return f.right[i]
}

// denied lists lexers we do not use: Jungle loops forever on ordinary text (chroma v2.14.0), and
// Markdown hands its code blocks to other lexers, which would get Jungle back in by the side door.
var denied = map[string]bool{"jungle": true, "markdown": true}

// Lexers are third-party code run on hostile text, and a lexer that is stuck cannot be stopped. So
// at most maxRunning of them may run at once (stuck ones keep their slot, which stops us feeding
// the loop), and Code is held to a time budget that refills as time passes.
const (
	maxRunning = 2
	codeBudget = 600 * time.Millisecond
	codeMin    = 50 * time.Millisecond
)

var (
	slots     = make(chan struct{}, maxRunning) // a token per lexer that is running
	tokenised atomic.Int32                      // how many times Code really ran a lexer; for the tests
	bucket    = &timeBudget{left: codeBudget, last: time.Now()}
)

type timeBudget struct {
	mu   sync.Mutex
	left time.Duration
	last time.Time
}

func (b *timeBudget) refill() {
	now := time.Now()
	b.left = min(b.left+now.Sub(b.last), codeBudget)
	b.last = now
}

// take reports whether there is enough budget to start something.
func (b *timeBudget) take() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.refill()
	return b.left >= codeMin
}

func (b *timeBudget) spend(d time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.refill()
	b.left -= d
}

func (b *timeBudget) set(d time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.left, b.last = d, time.Now()
}

// withLimit runs fn on its own goroutine and waits at most limit for it. A panic, a timeout, or no
// free slot gives ok == false.
func withLimit[T any](limit time.Duration, fn func() T) (out T, ok bool) {
	select {
	case slots <- struct{}{}:
	default:
		return out, false
	}
	start := time.Now()
	done := make(chan T, 1)
	go func() {
		defer func() { <-slots }()
		defer func() {
			if recover() != nil {
				var zero T
				done <- zero
			}
		}()
		done <- fn()
	}()
	select {
	case v := <-done:
		if time.Since(start) > limit { // both were ready: the budget was spent, so it is a timeout
			return out, false
		}
		return v, true
	case <-time.After(limit):
		return out, false
	}
}

// validLang accepts only names that look like a language: short, ASCII letters, digits and _+#.-
// (a long or odd name makes chroma's name lookup slow).
func validLang(s string) bool {
	if s == "" || len(s) > 32 {
		return false
	}
	for _, c := range []byte(s) {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_', c == '+', c == '#', c == '.', c == '-':
		default:
			return false
		}
	}
	return true
}

var (
	codeMu    sync.Mutex
	codeCache = map[[32]byte][][]Seg{}
)

const maxCodeCache = 256

// ForFile highlights a file's diff within a second; see ForFileWithin.
func ForFile(f diff.File) *File { return ForFileWithin(f, time.Second) }

// Size is how many bytes of text ForFile would hand to the lexer.
func Size(f diff.File) int {
	n := 0
	for _, l := range f.Lines {
		if l.Kind != diff.Hunk && l.Kind != diff.Meta {
			n += len(l.Text)
		}
	}
	return n
}

// ForFileWithin highlights a file's diff, or returns nil when its language is unknown, it is binary
// or too big, the lexer fails or panics, or it takes longer than limit. Lexers are third-party code
// run on hostile text: a lexer that does not finish is abandoned and the file stays plain.
func ForFileWithin(f diff.File, limit time.Duration) *File {
	if f.Binary || len(f.Lines) > maxFileLine {
		return nil
	}
	lexer := lexers.Match(f.Path)
	if lexer == nil || denied[strings.ToLower(lexer.Config().Name)] {
		return nil
	}
	h, _ := withLimit(limit, func() *File { return build(f, chroma.Coalesce(lexer)) })
	return h
}

func build(f diff.File, lexer chroma.Lexer) *File {
	style := styles.Get("monokai")
	out := &File{left: map[int][]Seg{}, right: map[int][]Seg{}}
	var oldIdx, newIdx []int
	flush := func() {
		side(f, lexer, style, oldIdx, out.left)
		side(f, lexer, style, newIdx, out.right)
		oldIdx, newIdx = nil, nil
	}
	for i, l := range f.Lines {
		switch l.Kind {
		case diff.Hunk:
			flush()
		case diff.Context:
			oldIdx, newIdx = append(oldIdx, i), append(newIdx, i)
		case diff.Del:
			oldIdx = append(oldIdx, i)
		case diff.Add:
			newIdx = append(newIdx, i)
		}
	}
	flush()
	return out
}

func expandTabs(s string) string { return strings.ReplaceAll(s, "\t", "    ") }

// tokenLines runs the lexer over text and returns the segments of every line.
func tokenLines(lexer chroma.Lexer, style *chroma.Style, text string) ([][]Seg, error) {
	plain := style.Get(chroma.Text)
	it, err := lexer.Tokenise(nil, text)
	if err != nil {
		return nil, err
	}
	lines := [][]Seg{nil}
	for _, tok := range it.Tokens() {
		entry := style.Get(tok.Type)
		fg := ""
		if entry.Colour.IsSet() && entry.Colour != plain.Colour { // plain text keeps the terminal's own colour
			fg = entry.Colour.String()
		}
		for k, piece := range strings.Split(tok.Value, "\n") {
			if k > 0 {
				lines = append(lines, nil)
			}
			if piece == "" {
				continue
			}
			lines[len(lines)-1] = append(lines[len(lines)-1], Seg{Text: expandTabs(piece), FG: fg, Bold: entry.Bold == chroma.Yes, Italic: entry.Italic == chroma.Yes})
		}
	}
	return lines, nil
}

// segsFor returns line n's segments when they add up to exactly the line's text, else nil.
func segsFor(lines [][]Seg, n int, text string) []Seg {
	if n >= len(lines) || len(text) > maxLineLen {
		return nil
	}
	var b strings.Builder
	for _, s := range lines[n] {
		b.WriteString(s.Text)
	}
	if b.String() == expandTabs(text) && b.Len() > 0 { // anything odd: leave the line plain
		return lines[n]
	}
	return nil
}

// side tokenises the lines idx of f as one text and stores each line's segments in into.
func side(f diff.File, lexer chroma.Lexer, style *chroma.Style, idx []int, into map[int][]Seg) {
	if len(idx) == 0 {
		return
	}
	texts := make([]string, len(idx))
	for n, i := range idx {
		if t := f.Lines[i].Text; len(t) <= maxLineLen {
			texts[n] = t
		}
	}
	lines, err := tokenLines(lexer, style, strings.Join(texts, "\n")+"\n")
	if err != nil {
		return
	}
	for n, i := range idx {
		if segs := segsFor(lines, n, f.Lines[i].Text); segs != nil {
			into[i] = segs
		}
	}
}

// Code colours a piece of code (a fenced block in some markdown) by language name or alias, one
// entry per line; a line that stays plain is nil. nil comes back for an unknown, empty or risky
// language, and when the lexer fails, panics or takes longer than limit.
func Code(lang, text string, limit time.Duration) [][]Seg {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if !validLang(lang) {
		return nil
	}
	key := sha256.Sum256([]byte(lang + "\x00" + text))
	codeMu.Lock()
	cached, ok := codeCache[key]
	codeMu.Unlock()
	if ok {
		return cached
	}
	lexer := lexers.Get(lang)
	if lexer == nil || denied[strings.ToLower(lexer.Config().Name)] || !bucket.take() {
		return nil
	}
	src := strings.Split(text, "\n")
	shown := make([]string, len(src))
	for i, l := range src {
		if len(l) <= maxLineLen {
			shown[i] = l
		}
	}
	start := time.Now()
	out, _ := withLimit(limit, func() [][]Seg {
		tokenised.Add(1)
		lines, err := tokenLines(chroma.Coalesce(lexer), styles.Get("monokai"), strings.Join(shown, "\n")+"\n")
		if err != nil {
			return nil
		}
		res := make([][]Seg, len(src))
		for i, l := range src {
			res[i] = segsFor(lines, i, l)
		}
		return res
	})
	bucket.spend(time.Since(start))
	if out != nil {
		codeMu.Lock()
		if len(codeCache) >= maxCodeCache {
			codeCache = map[[32]byte][][]Seg{}
		}
		codeCache[key] = out
		codeMu.Unlock()
	}
	return out
}
