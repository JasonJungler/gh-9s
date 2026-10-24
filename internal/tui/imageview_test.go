package tui

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/MurabitoB/gh-9s/internal/termimg"
)

type viewerRig struct {
	v        *imageViewer
	out      *bytes.Buffer
	restored int
	opened   []string
	fetched  []string
}

func newViewer(keysTyped string, fetchErr error) *viewerRig {
	r := &viewerRig{out: &bytes.Buffer{}}
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{200, 10, 10, 255})
		}
	}
	r.v = &imageViewer{
		host: "github.com", url: "https://h/p.png",
		in: strings.NewReader(keysTyped), out: r.out,
		fetch: func(host, url string) (image.Image, error) {
			r.fetched = append(r.fetched, host+" "+url)
			if fetchErr != nil {
				return nil, fetchErr
			}
			return img, nil
		},
		open:    func(u string) error { r.opened = append(r.opened, u); return nil },
		env:     termimg.Env{Get: func(string) string { return "" }, TmuxPassthrough: func() string { return "" }},
		size:    func() (int, int) { return 40, 20 },
		makeRaw: func() (func(), error) { return func() { r.restored++ }, nil },
	}
	return r
}

func TestViewerDrawsTheImageAndWaitsForEsc(t *testing.T) {
	r := newViewer("\x1b", nil)
	if err := r.v.Run(); err != nil {
		t.Fatal(err)
	}
	out := r.out.String()
	if !strings.Contains(out, "▀") || !strings.Contains(out, "esc") || !strings.Contains(out, "open in browser") {
		t.Errorf("viewer output lacks the picture or the hint: %q", out)
	}
	if len(r.fetched) != 1 || r.fetched[0] != "github.com https://h/p.png" {
		t.Errorf("fetched %v", r.fetched)
	}
	if r.restored != 1 {
		t.Errorf("terminal restored %d times; want once", r.restored)
	}
}

func TestViewerOOpensTheBrowserAndKeepsWaiting(t *testing.T) {
	r := newViewer("oq", nil)
	if err := r.v.Run(); err != nil {
		t.Fatal(err)
	}
	if len(r.opened) != 1 || r.opened[0] != "https://h/p.png" {
		t.Errorf("opened %v", r.opened)
	}
}

func TestViewerShowsTheErrorAndStillWaits(t *testing.T) {
	r := newViewer("q", errors.New("download refused (HTTP 404)"))
	if err := r.v.Run(); err != nil {
		t.Fatal(err)
	}
	out := r.out.String()
	if !strings.Contains(out, "download refused (HTTP 404)") || !strings.Contains(out, "open in browser") || strings.Contains(out, "▀") {
		t.Errorf("error screen = %q", out)
	}
	if r.restored != 1 {
		t.Errorf("terminal restored %d times", r.restored)
	}
}

func TestViewerEndsWhenInputEnds(t *testing.T) {
	r := newViewer("", nil)
	if err := r.v.Run(); err != nil {
		t.Fatal(err)
	}
}

func TestViewerRestoresTheTerminalEvenWhenDrawingPanics(t *testing.T) {
	r := newViewer("q", nil)
	r.v.fetch = func(string, string) (image.Image, error) { panic("boom") }
	func() {
		defer func() { _ = recover() }()
		_ = r.v.Run()
	}()
	if r.restored != 1 {
		t.Errorf("terminal restored %d times after a panic; want once", r.restored)
	}
}

func TestImagesOffOpensTheBrowserInstead(t *testing.T) {
	t.Setenv("GH9S_IMAGES", "off")
	opened := recordOpenURL(t)
	m, cmd := defaultShowImage(model{target: acme}, "https://h/p.png")
	if cmd != nil || len(*opened) != 1 || (*opened)[0] != "https://h/p.png" {
		t.Errorf("cmd=%v opened=%v; want no viewer and the browser", cmd != nil, *opened)
	}
	_ = m
}

func TestAfterTheViewerTheDetailIsExactlyAsItWas(t *testing.T) {
	m := prWithBody(t, strings.Repeat("filler\n\n", 40)+"![a](https://h/1.png)")
	m = keys(m, "tab")
	before := m.screenData(viewPRDetail).(prDetailState)
	next, _ := m.Update(imageDoneMsg{})
	after := next.(model).screenData(viewPRDetail).(prDetailState)
	if after.scroll != before.scroll || after.img != before.img || next.(model).view != viewPRDetail {
		t.Errorf("state changed across the viewer: %+v -> %+v", before, after)
	}
	next, _ = m.Update(imageDoneMsg{err: errors.New("terminal went away")})
	if st := next.(model).status; !strings.Contains(st, "terminal went away") {
		t.Errorf("status = %q; want the error", st)
	}
}
