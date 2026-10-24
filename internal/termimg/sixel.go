package termimg

import (
	"fmt"
	"image"
	"strconv"
	"strings"
)

// encodeSixel returns img as a Sixel sequence. Colours are snapped to the 6×6×6 cube, so the
// palette never exceeds 216 entries (maxColors above that changes nothing).
func encodeSixel(img image.Image, maxColors int) string {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return ""
	}
	// cube index of every pixel, and the palette in order of first appearance
	idx := make([]int, w*h)
	slot := map[int]int{}
	var palette []int
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, bl, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			c := xterm256(uint8(r>>8), uint8(g>>8), uint8(bl>>8)) - 16
			if _, ok := slot[c]; !ok {
				slot[c] = len(palette)
				palette = append(palette, c)
			}
			idx[y*w+x] = slot[c]
		}
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "\x1bPq\"1;1;%d;%d", w, h)
	for i, c := range palette {
		fmt.Fprintf(&sb, "#%d;2;%d;%d;%d", i, c/36*20, c/6%6*20, c%6*20)
	}
	for y0 := 0; y0 < h; y0 += 6 {
		for col := range palette {
			row := make([]byte, w)
			used := false
			for x := 0; x < w; x++ {
				var bits byte
				for k := 0; k < 6 && y0+k < h; k++ {
					if idx[(y0+k)*w+x] == col {
						bits |= 1 << k
					}
				}
				row[x] = '?' + bits
				used = used || bits != 0
			}
			if !used {
				continue
			}
			sb.WriteString("#" + strconv.Itoa(col))
			writeRuns(&sb, strings.TrimRight(string(row), "?"))
			sb.WriteString("$")
		}
		if y0+6 < h {
			sb.WriteString("-")
		}
	}
	sb.WriteString("\x1b\\")
	return sb.String()
}

// writeRuns writes s with runs of four or more identical characters as !n<char>.
func writeRuns(sb *strings.Builder, s string) {
	for i := 0; i < len(s); {
		j := i
		for j < len(s) && s[j] == s[i] {
			j++
		}
		if n := j - i; n >= 4 {
			sb.WriteString("!" + strconv.Itoa(n) + string(s[i]))
		} else {
			sb.WriteString(strings.Repeat(string(s[i]), n))
		}
		i = j
	}
}
