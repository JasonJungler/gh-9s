package tui

import (
	"fmt"
	"image"
	"io"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/term"

	"github.com/MurabitoB/gh-9s/internal/ghcli"
	"github.com/MurabitoB/gh-9s/internal/termimg"
)

// fetchImage downloads an image; tests replace it.
var fetchImage = ghcli.FetchImage

// showImage opens the full-screen image viewer; tests replace it.
var showImage = defaultShowImage

// imageDoneMsg arrives when the viewer has handed the terminal back.
type imageDoneMsg struct{ err error }

// defaultShowImage runs the viewer with the terminal released, so no graphics sequence ever goes
// through Bubble Tea's renderer. With GH9S_IMAGES=off the link opens in the browser instead.
func defaultShowImage(m model, url string) (model, tea.Cmd) {
	env := realImageEnv()
	if termimg.Detect(env) == termimg.Off {
		return m.open(url), nil
	}
	v := &imageViewer{host: m.curTarget().Host, url: url, fetch: fetchImage, open: openURL, env: env}
	return m, tea.Exec(v, func(err error) tea.Msg { return imageDoneMsg{err: err} })
}

func realImageEnv() termimg.Env {
	return termimg.Env{
		Get: os.Getenv,
		TmuxPassthrough: func() string {
			out, err := exec.Command("tmux", "show", "-gv", "allow-passthrough").Output()
			if err != nil {
				return ""
			}
			return strings.TrimSpace(string(out))
		},
	}
}

// imageViewer shows one image on the whole screen and waits for a key. It is a tea.ExecCommand:
// Bubble Tea gives it the terminal and takes it back when Run returns.
type imageViewer struct {
	host, url string
	in        io.Reader
	out       io.Writer
	fetch     func(host, url string) (image.Image, error)
	open      func(url string) error
	env       termimg.Env
	size      func() (cols, rows int)
	makeRaw   func() (restore func(), err error)
}

func (v *imageViewer) SetStdin(r io.Reader)  { v.in = r }
func (v *imageViewer) SetStdout(w io.Writer) { v.out = w }
func (v *imageViewer) SetStderr(io.Writer)   {}

func (v *imageViewer) Run() error {
	restore := func() {}
	if v.makeRaw != nil {
		if r, err := v.makeRaw(); err == nil {
			restore = r
		}
	} else if f, ok := v.in.(*os.File); ok && term.IsTerminal(f.Fd()) {
		if st, err := term.MakeRaw(f.Fd()); err == nil {
			restore = func() { _ = term.Restore(f.Fd(), st) }
		}
	}
	defer restore()
	fmt.Fprint(v.out, "\x1b[?1049h") // the alternate screen: nothing is left on the user's shell screen
	defer fmt.Fprint(v.out, "\x1b[?1049l")

	cols, rows := 80, 24
	if v.size != nil {
		cols, rows = v.size()
	} else if f, ok := v.out.(*os.File); ok {
		if w, h, err := term.GetSize(f.Fd()); err == nil {
			cols, rows = w, h
		}
	}
	hint := "esc/q back · o open in browser"
	fmt.Fprint(v.out, "\x1b[2J\x1b[Hdownloading…")
	img, err := v.fetch(v.host, v.url)
	fmt.Fprint(v.out, "\x1b[2J\x1b[H")
	if err != nil {
		fmt.Fprintf(v.out, "can't show this image: %s\r\n%s\r\n\r\n%s\r\n", err, v.url, hint)
	} else {
		pic := termimg.Draw(img, termimg.Detect(v.env), cols, max(rows-3, 1), v.env)
		fmt.Fprintf(v.out, "%s\r\n\r\n%s\r\n", strings.ReplaceAll(pic, "\n", "\r\n"), hint)
	}
	buf := make([]byte, 1)
	for {
		if n, err := v.in.Read(buf); n == 0 || err != nil {
			return nil // input ended: nothing more to wait for
		}
		switch buf[0] {
		case 0x1b, 'q', '\r', '\n', 3:
			return nil
		case 'o':
			if link, err := ghcli.ResolveImageURL(v.host, v.url); err == nil && v.open != nil {
				_ = v.open(link)
			}
		}
	}
}
