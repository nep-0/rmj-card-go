package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/xml"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"strconv"
	"strings"
	"sync"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// These fonts include Chinese glyphs so PNG output is independent of host fonts.
//
//go:embed fonts/NotoSansSC-Regular.otf
var regularFont []byte

//go:embed fonts/NotoSansSC-Bold.otf
var boldFont []byte

var cardFonts = sync.OnceValues(func() ([2]*opentype.Font, error) {
	regular, err := opentype.Parse(regularFont)
	if err != nil {
		return [2]*opentype.Font{}, fmt.Errorf("parse regular font: %w", err)
	}
	bold, err := opentype.Parse(boldFont)
	if err != nil {
		return [2]*opentype.Font{}, fmt.Errorf("parse bold font: %w", err)
	}
	return [2]*opentype.Font{regular, bold}, nil
})

type cardText struct {
	value string
	attrs map[string]string
}

// splitCardSVG extracts text for font rendering; oksvg handles the remaining shapes.
func splitCardSVG(svg string) (string, []cardText, map[string]map[string]string, error) {
	decoder := xml.NewDecoder(strings.NewReader(svg))
	styles := make(map[string]map[string]string)
	var texts []cardText
	var shapes strings.Builder
	last := 0
	for {
		startOffset := int(decoder.InputOffset())
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", nil, nil, fmt.Errorf("parse card SVG: %w", err)
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		switch start.Name.Local {
		case "style":
			var css string
			if err := decoder.DecodeElement(&css, &start); err != nil {
				return "", nil, nil, fmt.Errorf("parse card styles: %w", err)
			}
			for _, rule := range strings.Split(css, "}") {
				selectors, declarations, ok := strings.Cut(rule, "{")
				if !ok {
					continue
				}
				for _, selector := range strings.Split(selectors, ",") {
					selector = strings.TrimSpace(selector)
					if styles[selector] == nil {
						styles[selector] = make(map[string]string)
					}
					applyDeclarations(styles[selector], declarations)
				}
			}
		case "text":
			var value string
			if err := decoder.DecodeElement(&value, &start); err != nil {
				return "", nil, nil, fmt.Errorf("parse card text: %w", err)
			}
			attrs := make(map[string]string)
			for _, attr := range start.Attr {
				attrs[attr.Name.Local] = attr.Value
			}
			texts = append(texts, cardText{value: value, attrs: attrs})
			shapes.WriteString(svg[last:startOffset])
			last = int(decoder.InputOffset())
		}
	}
	shapes.WriteString(svg[last:])
	return shapes.String(), texts, styles, nil
}

func applyDeclarations(attrs map[string]string, declarations string) {
	for _, declaration := range strings.Split(declarations, ";") {
		key, value, ok := strings.Cut(declaration, ":")
		if ok {
			attrs[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
}

func renderPNG(ctx context.Context, svg string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	svg, avatar, err := extractAvatar(svg)
	if err != nil {
		return nil, fmt.Errorf("extract card avatar: %w", err)
	}
	shapes, texts, styles, err := splitCardSVG(svg)
	if err != nil {
		return nil, err
	}
	// oksvg does not default ry to rx as SVG requires for rounded rectangles.
	shapes = strings.NewReplacer(` rx="8"`, ` rx="8" ry="8"`, ` rx="10"`, ` rx="10" ry="10"`).Replace(shapes)
	icon, err := oksvg.ReadIconStream(strings.NewReader(shapes))
	if err != nil {
		return nil, fmt.Errorf("parse card shapes: %w", err)
	}
	canvas := image.NewRGBA(image.Rect(0, 0, cardWidth, cardHeight))
	icon.SetTarget(0, 0, cardWidth, cardHeight)
	scanner := rasterx.NewScannerGV(cardWidth, cardHeight, canvas, canvas.Bounds())
	icon.Draw(rasterx.NewDasher(cardWidth, cardHeight, scanner), 1)
	if avatar != nil {
		draw.Draw(canvas, image.Rect(34, 72, 150, 188), avatar, avatar.Bounds().Min, draw.Over)
	}
	fonts, err := cardFonts()
	if err != nil {
		return nil, err
	}
	regular, bold := fonts[0], fonts[1]
	type faceKey struct {
		size float64
		bold bool
	}
	faces := make(map[faceKey]font.Face)
	defer func() {
		for _, face := range faces {
			face.Close()
		}
	}()
	for _, text := range texts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		attrs := map[string]string{"font-size": "16", "fill": "#000"}
		for key, value := range styles["text"] {
			attrs[key] = value
		}
		for _, class := range strings.Fields(text.attrs["class"]) {
			for key, value := range styles["."+class] {
				attrs[key] = value
			}
		}
		for key, value := range text.attrs {
			attrs[key] = value
		}
		applyDeclarations(attrs, text.attrs["style"])
		size, err := strconv.ParseFloat(strings.TrimSuffix(attrs["font-size"], "px"), 64)
		if err != nil || size <= 0 {
			return nil, fmt.Errorf("invalid text font size %q", attrs["font-size"])
		}
		key := faceKey{size: size, bold: attrs["font-weight"] == "700" || attrs["font-weight"] == "bold"}
		face := faces[key]
		if face == nil {
			selected := regular
			if key.bold {
				selected = bold
			}
			face, err = opentype.NewFace(selected, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingNone})
			if err != nil {
				return nil, fmt.Errorf("create card font: %w", err)
			}
			faces[key] = face
		}
		x, err := strconv.ParseFloat(attrs["x"], 64)
		if err != nil {
			return nil, fmt.Errorf("parse text x: %w", err)
		}
		y, err := strconv.ParseFloat(attrs["y"], 64)
		if err != nil {
			return nil, fmt.Errorf("parse text y: %w", err)
		}
		width := font.MeasureString(face, text.value)
		dot := fixed.Point26_6{X: fixed.Int26_6(x * 64), Y: fixed.Int26_6(y * 64)}
		switch attrs["text-anchor"] {
		case "middle":
			dot.X -= width / 2
		case "end":
			dot.X -= width
		}
		fill, err := textColor(attrs["fill"])
		if err != nil {
			return nil, err
		}
		drawer := font.Drawer{Dst: canvas, Src: image.NewUniform(fill), Face: face, Dot: dot}
		drawer.DrawString(text.value)
	}
	var output bytes.Buffer
	if err := png.Encode(&output, canvas); err != nil {
		return nil, fmt.Errorf("encode card PNG: %w", err)
	}
	return output.Bytes(), nil
}

func textColor(value string) (color.RGBA, error) {
	value = strings.TrimPrefix(value, "#")
	if len(value) == 3 {
		value = string([]byte{value[0], value[0], value[1], value[1], value[2], value[2]})
	}
	if len(value) != 6 {
		return color.RGBA{}, fmt.Errorf("unsupported card text color %q", value)
	}
	rgb, err := strconv.ParseUint(value, 16, 24)
	if err != nil {
		return color.RGBA{}, fmt.Errorf("parse text color: %w", err)
	}
	return color.RGBA{R: uint8(rgb >> 16), G: uint8(rgb >> 8), B: uint8(rgb), A: 255}, nil
}
