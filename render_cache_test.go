package main

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRenderCacheHitAndSingleflight(t *testing.T) {
	cache := newRenderCache(2, 16, time.Minute)
	var calls atomic.Int32
	start := make(chan struct{})
	render := func(context.Context) ([]byte, error) { calls.Add(1); <-start; return []byte("image"), nil }
	var wg sync.WaitGroup
	results := make(chan []byte, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			data, err := cache.load(t.Context(), "player\x00modern", render)
			if err != nil {
				t.Error(err)
				return
			}
			results <- data
		}()
	}
	for calls.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	close(start)
	wg.Wait()
	close(results)
	for data := range results {
		if string(data) != "image" {
			t.Errorf("cached data = %q", data)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("render calls = %d, want 1", calls.Load())
	}
	if _, err := cache.load(t.Context(), "player\x00modern", render); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Errorf("cache miss after fill: calls=%d", calls.Load())
	}
}

func TestRenderCacheByteBound(t *testing.T) {
	cache := newRenderCache(2, 5, time.Minute)
	render := func(data string) func(context.Context) ([]byte, error) {
		return func(context.Context) ([]byte, error) { return []byte(data), nil }
	}
	if _, err := cache.load(t.Context(), "large", render("123456")); err != nil {
		t.Fatal(err)
	}
	if _, ok := cache.get("large"); ok {
		t.Error("oversized image was cached")
	}
	if _, err := cache.load(t.Context(), "small", render("12345")); err != nil {
		t.Fatal(err)
	}
	if _, ok := cache.get("small"); !ok {
		t.Error("within-limit image was not cached")
	}
}
