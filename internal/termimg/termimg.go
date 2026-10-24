// Package termimg picks a way to show an image in the user's terminal and draws it. Detection looks
// at the environment only: it never asks the terminal anything.
package termimg

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"strings"
)

// Method is how an image reaches the screen.
type Method int

const (
	Off    Method = iota // no images
	Blocks               // half-block characters with colour; plain text, works anywhere
	ITerm2               // iTerm2 inline images (OSC 1337)
	Sixel                // Sixel graphics
)

func (m Method) String() string { return [...]string{"off", "blocks", "iterm2", "sixel"}[m] }

// Env is the outside world Detect and Draw look at.
type Env struct {
	Get             func(string) string // environment variables
	TmuxPassthrough func() string       // tmux's allow-passthrough setting: "on", "all", "off" or ""
}

// Detect chooses the method: GH9S_IMAGES first, then what the environment says about the terminal.
// Anything doubtful ends in Blocks.
func Detect(e Env) Method {
	switch strings.ToLower(e.Get("GH9S_IMAGES")) {
	case "off":
		return Off
	case "blocks":
		return Blocks
	case "iterm", "iterm2":
		return ITerm2
	case "sixel":
		return Sixel
	}
	if e.Get("TMUX") == "" && (e.Get("STY") != "" || strings.HasPrefix(e.Get("TERM"), "screen")) {
		return Blocks
	}
	if e.Get("TMUX") != "" {
		if pt := e.TmuxPassthrough(); pt != "on" && pt != "all" {
			return Blocks
		}
	}
	switch {
	case e.Get("TERM_PROGRAM") == "iTerm.app", e.Get("LC_TERMINAL") == "iTerm2":
		return ITerm2
	case e.Get("WT_SESSION") != "":
		return Sixel
	}
	return Blocks
}

// Fit returns the size in cells (columns, rows) an image of w×h pixels takes inside cols×rows
// cells, keeping its proportions. A cell is taken to be twice as tall as it is wide.
func Fit(w, h, cols, rows int) (c, r int) {
	w, h, cols, rows = max(w, 1), max(h, 1), max(cols, 1), max(rows, 1)
	c = cols
	r = ceilDiv(c*h, 2*w)
	if r > rows {
		r = rows
		c = ceilDiv(r*2*w, h)
	}
	return max(min(c, cols), 1), max(r, 1)
}

func ceilDiv(a, b int) int { return (a + b - 1) / b }

// Draw returns what to write to the terminal to show img in about cols×rows cells (cursor
// movement not included). Blocks come back as lines joined with "\n".
func Draw(img image.Image, m Method, cols, rows int, e Env) string {
	b := img.Bounds()
	c, r := Fit(b.Dx(), b.Dy(), cols, rows)
	switch m {
	case Blocks:
		return drawBlocks(img, c, r, trueColor(e))
	case ITerm2:
		return wrapTmux(iterm2(img, c, r), e)
	case Sixel:
		return wrapTmux(encodeSixel(scale(img, c*8, r*16), 256), e)
	}
	return ""
}

func trueColor(e Env) bool {
	v := strings.ToLower(e.Get("COLORTERM"))
	return v == "truecolor" || v == "24bit"
}

func wrapTmux(s string, e Env) string {
	if e.Get("TMUX") == "" || s == "" {
		return s
	}
	return "\x1bPtmux;" + strings.ReplaceAll(s, "\x1b", "\x1b\x1b") + "\x1b\\"
}

func iterm2(img image.Image, c, r int) string {
	b := img.Bounds()
	if b.Dx() > 1600 { // no point sending more pixels than a screen shows
		img = scale(img, 1600, max(b.Dy()*1600/b.Dx(), 1))
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return ""
	}
	return fmt.Sprintf("\x1b]1337;File=inline=1;width=%d;height=%d;preserveAspectRatio=1:%s\a",
		c, r, base64.StdEncoding.EncodeToString(buf.Bytes()))
}

func drawBlocks(img image.Image, c, r int, true24 bool) string {
	px := scale(img, c, r*2)
	var lines []string
	for y := 0; y < r; y++ {
		var sb strings.Builder
		for x := 0; x < c; x++ {
			top, bot := px.RGBAAt(x, 2*y), px.RGBAAt(x, 2*y+1)
			if true24 {
				fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%d;48;2;%d;%d;%dm▀", top.R, top.G, top.B, bot.R, bot.G, bot.B)
			} else {
				fmt.Fprintf(&sb, "\x1b[38;5;%d;48;5;%dm▀", xterm256(top.R, top.G, top.B), xterm256(bot.R, bot.G, bot.B))
			}
		}
		sb.WriteString("\x1b[0m")
		lines = append(lines, sb.String())
	}
	return strings.Join(lines, "\n")
}

// xterm256 maps a colour to the 6×6×6 cube of the 256-colour palette.
func xterm256(r, g, b uint8) int {
	q := func(v uint8) int { return (int(v)*5 + 127) / 255 }
	return 16 + 36*q(r) + 6*q(g) + q(b)
}

// scale resizes img to w×h by averaging the source pixels under each target pixel; transparent
// pixels count as black.
func scale(img image.Image, w, h int) *image.RGBA {
	w, h = max(w, 1), max(h, 1)
	b := img.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		y0, y1 := b.Min.Y+y*b.Dy()/h, b.Min.Y+(y+1)*b.Dy()/h
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < w; x++ {
			x0, x1 := b.Min.X+x*b.Dx()/w, b.Min.X+(x+1)*b.Dx()/w
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var sr, sg, sb, n uint64
			for yy := y0; yy < y1 && yy < b.Max.Y; yy++ {
				for xx := x0; xx < x1 && xx < b.Max.X; xx++ {
					r, g, bl, _ := img.At(xx, yy).RGBA()
					sr, sg, sb, n = sr+uint64(r>>8), sg+uint64(g>>8), sb+uint64(bl>>8), n+1
				}
			}
			if n > 0 {
				out.Pix[out.PixOffset(x, y)+0] = uint8(sr / n)
				out.Pix[out.PixOffset(x, y)+1] = uint8(sg / n)
				out.Pix[out.PixOffset(x, y)+2] = uint8(sb / n)
			}
			out.Pix[out.PixOffset(x, y)+3] = 255
		}
	}
	return out
}
