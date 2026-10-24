package termimg

import (
	"image"
	"image/color"
	"strings"
	"testing"
)

func env(vars map[string]string, passthrough string) Env {
	return Env{Get: func(k string) string { return vars[k] }, TmuxPassthrough: func() string { return passthrough }}
}

func TestDetect(t *testing.T) {
	cases := []struct {
		name string
		vars map[string]string
		pt   string
		want Method
	}{
		{"nothing set", nil, "", Blocks},
		{"iterm", map[string]string{"TERM_PROGRAM": "iTerm.app"}, "", ITerm2},
		{"iterm over ssh", map[string]string{"LC_TERMINAL": "iTerm2"}, "", ITerm2},
		{"windows terminal", map[string]string{"WT_SESSION": "abc"}, "", Sixel},
		{"apple terminal", map[string]string{"TERM_PROGRAM": "Apple_Terminal"}, "", Blocks},
		{"vscode", map[string]string{"TERM_PROGRAM": "vscode"}, "", Blocks},
		{"off", map[string]string{"GH9S_IMAGES": "off", "TERM_PROGRAM": "iTerm.app"}, "", Off},
		{"forced blocks", map[string]string{"GH9S_IMAGES": "blocks", "TERM_PROGRAM": "iTerm.app"}, "", Blocks},
		{"forced iterm", map[string]string{"GH9S_IMAGES": "iterm"}, "", ITerm2},
		{"forced sixel", map[string]string{"GH9S_IMAGES": "sixel"}, "", Sixel},
		{"auto spelled out", map[string]string{"GH9S_IMAGES": "auto", "WT_SESSION": "x"}, "", Sixel},
		{"unknown value is auto", map[string]string{"GH9S_IMAGES": "bogus", "WT_SESSION": "x"}, "", Sixel},
		{"tmux without passthrough", map[string]string{"TMUX": "/tmp/t,1,0", "LC_TERMINAL": "iTerm2"}, "off", Blocks},
		{"tmux with passthrough on", map[string]string{"TMUX": "/tmp/t,1,0", "LC_TERMINAL": "iTerm2"}, "on", ITerm2},
		{"tmux with passthrough all", map[string]string{"TMUX": "/tmp/t,1,0", "WT_SESSION": "x"}, "all", Sixel},
		{"tmux unknown terminal", map[string]string{"TMUX": "/tmp/t,1,0", "TERM_PROGRAM": "tmux"}, "on", Blocks},
		{"tmux forced", map[string]string{"TMUX": "/tmp/t,1,0", "GH9S_IMAGES": "iterm"}, "off", ITerm2},
		{"screen", map[string]string{"STY": "123.pts", "TERM_PROGRAM": "iTerm.app"}, "", Blocks},
		{"screen term", map[string]string{"TERM": "screen-256color", "LC_TERMINAL": "iTerm2"}, "", Blocks},
	}
	for _, c := range cases {
		if got := Detect(env(c.vars, c.pt)); got != c.want {
			t.Errorf("%s: Detect = %v; want %v", c.name, got, c.want)
		}
	}
}

func TestFit(t *testing.T) {
	cases := []struct{ w, h, cols, rows, wc, wr int }{
		{400, 100, 10, 10, 10, 2},
		{100, 400, 10, 10, 5, 10},
		{100, 100, 20, 5, 10, 5},
		{1, 1, 10, 10, 10, 5},
	}
	for _, c := range cases {
		if cc, rr := Fit(c.w, c.h, c.cols, c.rows); cc != c.wc || rr != c.wr {
			t.Errorf("Fit(%d,%d,%d,%d) = %d,%d; want %d,%d", c.w, c.h, c.cols, c.rows, cc, rr, c.wc, c.wr)
		}
	}
}

func redOverBlue() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for x := 0; x < 2; x++ {
		img.Set(x, 0, color.RGBA{255, 0, 0, 255})
		img.Set(x, 1, color.RGBA{0, 0, 255, 255})
	}
	return img
}

func TestDrawBlocks(t *testing.T) {
	true24 := env(map[string]string{"COLORTERM": "truecolor"}, "")
	got := Draw(redOverBlue(), Blocks, 2, 1, true24)
	want := "\x1b[38;2;255;0;0;48;2;0;0;255m▀\x1b[38;2;255;0;0;48;2;0;0;255m▀\x1b[0m"
	if got != want {
		t.Errorf("truecolor blocks = %q; want %q", got, want)
	}
	got = Draw(redOverBlue(), Blocks, 2, 1, env(nil, ""))
	want = "\x1b[38;5;196;48;5;21m▀\x1b[38;5;196;48;5;21m▀\x1b[0m"
	if got != want {
		t.Errorf("256-colour blocks = %q; want %q", got, want)
	}
	two := Draw(image.NewRGBA(image.Rect(0, 0, 2, 4)), Blocks, 2, 4, true24)
	if strings.Count(two, "\n") != 1 {
		t.Errorf("two rows should be joined by one newline: %q", two)
	}
}

func TestDrawITerm2(t *testing.T) {
	got := Draw(redOverBlue(), ITerm2, 4, 2, env(nil, ""))
	if !strings.HasPrefix(got, "\x1b]1337;File=inline=1;width=4;height=2;preserveAspectRatio=1:") || !strings.HasSuffix(got, "\a") {
		t.Errorf("iTerm2 sequence = %q", got)
	}
}

func TestDrawInsideTmuxWrapsThePassThrough(t *testing.T) {
	got := Draw(redOverBlue(), ITerm2, 4, 2, env(map[string]string{"TMUX": "x"}, "on"))
	if !strings.HasPrefix(got, "\x1bPtmux;") || !strings.HasSuffix(got, "\x1b\\") {
		t.Fatalf("not wrapped: %q", got[:20])
	}
	inner := strings.TrimSuffix(strings.TrimPrefix(got, "\x1bPtmux;"), "\x1b\\")
	if !strings.HasPrefix(inner, "\x1b\x1b]1337;File=") {
		t.Errorf("the ESC inside must be doubled: %q", inner[:20])
	}
	blocks := Draw(redOverBlue(), Blocks, 2, 1, env(map[string]string{"TMUX": "x"}, "on"))
	if strings.HasPrefix(blocks, "\x1bPtmux;") {
		t.Error("plain text blocks must not be wrapped")
	}
}

func TestDrawOffIsEmpty(t *testing.T) {
	if got := Draw(redOverBlue(), Off, 2, 1, env(nil, "")); got != "" {
		t.Errorf("Off drew %q", got)
	}
}
