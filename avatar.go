package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg" // QQ avatars may be JPEG.
	"image/png"
	"io"
	"net/http"
	"strings"
	"time"

	xdraw "golang.org/x/image/draw"
)

var avatarClient = &http.Client{Timeout: 5 * time.Second}

func fetchAvatar(ctx context.Context, client *http.Client, qq string) ([]byte, error) {
	if len(qq) < 5 || len(qq) > 12 {
		return nil, nil
	}
	for _, digit := range qq {
		if digit < '0' || digit > '9' {
			return nil, nil
		}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://q1.qlogo.cn/g?b=qq&s=640&nk="+qq, nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.HasPrefix(response.Header.Get("Content-Type"), "image/") {
		return nil, fmt.Errorf("avatar response status/type: %d/%q", response.StatusCode, response.Header.Get("Content-Type"))
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 2_000_001))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 || len(data) > 2_000_000 {
		return nil, fmt.Errorf("avatar exceeds size limit or is empty")
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode avatar dimensions: %w", err)
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > 4096 || config.Height > 4096 {
		return nil, fmt.Errorf("avatar dimensions out of bounds")
	}
	source, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode avatar: %w", err)
	}
	bounds := source.Bounds()
	size := min(bounds.Dx(), bounds.Dy())
	crop := image.Rect(bounds.Min.X+(bounds.Dx()-size)/2, bounds.Min.Y+(bounds.Dy()-size)/2, bounds.Min.X+(bounds.Dx()+size)/2, bounds.Min.Y+(bounds.Dy()+size)/2)
	scaled := image.NewRGBA(image.Rect(0, 0, 116, 116))
	xdraw.CatmullRom.Scale(scaled, scaled.Bounds(), source, crop, draw.Src, nil)
	mask := image.NewAlpha(scaled.Bounds())
	for y := range 116 {
		for x := range 116 {
			coverage := 0
			for sy := range 4 {
				for sx := range 4 {
					dx := float64(x) + float64(sx)/4 + .125 - 58
					dy := float64(y) + float64(sy)/4 + .125 - 58
					if dx*dx+dy*dy <= 58*58 {
						coverage++
					}
				}
			}
			mask.SetAlpha(x, y, color.Alpha{A: uint8(coverage * 255 / 16)})
		}
	}
	result := image.NewRGBA(scaled.Bounds())
	draw.DrawMask(result, result.Bounds(), scaled, image.Point{}, mask, image.Point{}, draw.Src)
	var output bytes.Buffer
	if err := png.Encode(&output, result); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func embedAvatar(svg string, avatar []byte) string {
	if len(avatar) == 0 {
		return svg
	}
	markup := `<image x="34" y="72" width="116" height="116" href="data:image/png;base64,` + base64.StdEncoding.EncodeToString(avatar) + `"/><circle cx="92" cy="130" r="58" fill="none" stroke="#dbe4f3" stroke-width="3"/>`
	return strings.Replace(svg, `<circle cx="92" cy="130" r="58" fill="#dbe4f3"/>`, markup, 1)
}

// extractAvatar removes the embedded bitmap before the vector-only raster pass.
func extractAvatar(svg string) (string, image.Image, error) {
	decoder := xml.NewDecoder(strings.NewReader(svg))
	for {
		offset := int(decoder.InputOffset())
		token, err := decoder.Token()
		if err == io.EOF {
			return svg, nil, nil
		}
		if err != nil {
			return "", nil, err
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "image" {
			continue
		}
		var href string
		for _, attr := range start.Attr {
			if attr.Name.Local == "href" {
				href = attr.Value
			}
		}
		if err := decoder.Skip(); err != nil {
			return "", nil, err
		}
		encoded, ok := strings.CutPrefix(href, "data:image/png;base64,")
		if !ok {
			return "", nil, fmt.Errorf("unsupported avatar image source")
		}
		data, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return "", nil, err
		}
		avatar, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			return "", nil, err
		}
		return svg[:offset] + svg[int(decoder.InputOffset()):], avatar, nil
	}
}
