package termimg

import (
	"image"
	"image/color"
	"strings"
	"testing"
)

func solid(w, h int, c color.RGBA) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

func TestSixelTwoColours(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 6, 6))
	for y := 0; y < 6; y++ {
		for x := 0; x < 6; x++ {
			if x < 3 {
				img.Set(x, y, color.RGBA{255, 0, 0, 255})
			} else {
				img.Set(x, y, color.RGBA{0, 0, 255, 255})
			}
		}
	}
	got := encodeSixel(img, 256)
	for _, want := range []string{"\x1bPq\"1;1;6;6", "#0;2;100;0;0", "#1;2;0;0;100", "#0~~~", "#1???~~~", "\x1b\\"} {
		if !strings.Contains(got, want) {
			t.Errorf("sixel lacks %q:\n%q", want, got)
		}
	}
	if !strings.HasSuffix(got, "\x1b\\") {
		t.Errorf("not terminated: %q", got)
	}
}

func TestSixelRunLengthAndBands(t *testing.T) {
	got := encodeSixel(solid(10, 12, color.RGBA{255, 0, 0, 255}), 256)
	if strings.Count(got, "#0;2;") != 1 {
		t.Errorf("one colour should give one palette entry: %q", got)
	}
	if !strings.Contains(got, "#0!10~") {
		t.Errorf("a run of ten identical sixels should be run-length coded: %q", got)
	}
	if strings.Count(got, "-") != 1 {
		t.Errorf("12 rows are two bands of six, separated by one '-': %q", got)
	}
}

func TestSixelPartialLastBand(t *testing.T) {
	got := encodeSixel(solid(2, 7, color.RGBA{0, 255, 0, 255}), 256)
	// seven rows: a full band (~) then one row (bit 0 only: '@')
	if !strings.Contains(got, "!2~") && !strings.Contains(got, "~~") || !strings.Contains(got, "@") {
		t.Errorf("last band should hold a single row: %q", got)
	}
}

func TestDrawSixelScalesAndTerminates(t *testing.T) {
	got := Draw(solid(100, 100, color.RGBA{1, 2, 3, 255}), Sixel, 4, 2, env(nil, ""))
	if !strings.HasPrefix(got, "\x1bPq") || !strings.HasSuffix(got, "\x1b\\") {
		t.Errorf("sixel = %q", got[:20])
	}
}
