package main

import (
	"bytes"
	"image/png"
	"testing"
)

func TestPNGEmbeddedChineseText(t *testing.T) {
	// No executable or installed font lookup may be needed for PNG output.
	t.Setenv("PATH", "")
	t.Setenv("PROGRAMFILES", "")
	t.Setenv("PROGRAMFILES(X86)", "")
	t.Setenv("LOCALAPPDATA", "")
	data, err := renderPNG(t.Context(), `<svg xmlns="http://www.w3.org/2000/svg" width="1280" height="1100"><rect width="1280" height="1100" fill="#fff"/><text x="20" y="60" font-size="36" fill="#123456">火箭</text></svg>`)
	if err != nil {
		t.Fatal(err)
	}
	canvas, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	for _, region := range [][2]int{{20, 56}, {56, 92}} {
		ink := 0
		for y := 20; y < 65; y++ {
			for x := region[0]; x < region[1]; x++ {
				r, g, b, _ := canvas.At(x, y).RGBA()
				if r < 0x8000 && g < 0x8000 && b < 0x8000 {
					ink++
				}
			}
		}
		if ink < 50 {
			t.Errorf("Chinese glyph in x=%v has %d dark pixels; text was not rendered", region, ink)
		}
	}
}
