package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAvatarCard(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 32, 16))
	for y := range 16 {
		for x := range 32 {
			source.Set(x, y, color.RGBA{R: 240, A: 255})
		}
	}
	var data bytes.Buffer
	if err := png.Encode(&data, source); err != nil {
		t.Fatal(err)
	}
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Get("nk") != "123456" {
			t.Errorf("QQ query = %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(data.Bytes())
	}))
	defer server.Close()
	client := server.Client()
	client.Transport = &avatarTransport{base: client.Transport, url: server.URL}
	avatar, err := fetchAvatar(t.Context(), client, "123456")
	if err != nil {
		t.Fatal(err)
	}
	for _, style := range []string{"modern", "QH"} {
		svg := embedAvatar(renderCard(fakeStatistics{}.player, style, ""), avatar)
		output, err := renderPNG(t.Context(), svg)
		if err != nil {
			t.Fatal(err)
		}
		canvas, err := png.Decode(bytes.NewReader(output))
		if err != nil {
			t.Fatal(err)
		}
		r, g, _, _ := canvas.At(92, 130).RGBA()
		if r < 0xd000 || g > 0x1000 {
			t.Errorf("%s avatar center = %v", style, canvas.At(92, 130))
		}
		r, g, b, _ := canvas.At(34, 72).RGBA()
		if r < 0xf000 || g < 0xf000 || b < 0xf000 {
			t.Errorf("%s avatar corner not clipped: %v", style, canvas.At(34, 72))
		}
	}
	if _, err := fetchAvatar(t.Context(), client, "invalid"); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Errorf("avatar requests = %d, want 1", calls)
	}
}

type avatarTransport struct {
	base http.RoundTripper
	url  string
}

func (a *avatarTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	clone := r.Clone(r.Context())
	target, err := http.NewRequest(r.Method, a.url+r.URL.RequestURI(), nil)
	if err != nil {
		return nil, err
	}
	clone.URL = target.URL
	return a.base.RoundTrip(clone)
}
