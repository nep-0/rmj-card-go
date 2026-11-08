package main

import (
	"context"
	"sync"
	"time"
)

type renderCacheEntry struct {
	data    []byte
	expires time.Time
}
type renderCall struct {
	done chan struct{}
	data []byte
	err  error
}
type renderCache struct {
	mu                          sync.Mutex
	entries                     map[string]renderCacheEntry
	calls                       map[string]*renderCall
	maxEntries, maxBytes, bytes int
	ttl                         time.Duration
}

func newRenderCache(maxEntries, maxBytes int, ttl time.Duration) *renderCache {
	return &renderCache{entries: make(map[string]renderCacheEntry), calls: make(map[string]*renderCall), maxEntries: maxEntries, maxBytes: maxBytes, ttl: ttl}
}
func (c *renderCache) get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.expires) {
		delete(c.entries, key)
		c.bytes -= len(entry.data)
		return nil, false
	}
	return entry.data, true
}
func (c *renderCache) load(ctx context.Context, key string, render func(context.Context) ([]byte, error)) ([]byte, error) {
	if data, ok := c.get(key); ok {
		return data, nil
	}
	c.mu.Lock()
	if call := c.calls[key]; call != nil {
		c.mu.Unlock()
		select {
		case <-call.done:
			return call.data, call.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	call := &renderCall{done: make(chan struct{})}
	c.calls[key] = call
	c.mu.Unlock()
	call.data, call.err = render(ctx)
	c.mu.Lock()
	if call.err == nil && len(call.data) <= c.maxBytes && c.maxEntries > 0 && c.maxBytes > 0 {
		c.expireLocked()
		for len(c.entries) >= c.maxEntries || c.bytes+len(call.data) > c.maxBytes {
			c.evictLocked()
		}
		copyData := append([]byte(nil), call.data...)
		c.entries[key] = renderCacheEntry{copyData, time.Now().Add(c.ttl)}
		c.bytes += len(copyData)
	}
	delete(c.calls, key)
	close(call.done)
	c.mu.Unlock()
	return call.data, call.err
}
func (c *renderCache) expireLocked() {
	now := time.Now()
	for key, entry := range c.entries {
		if now.After(entry.expires) {
			delete(c.entries, key)
			c.bytes -= len(entry.data)
		}
	}
}
func (c *renderCache) evictLocked() {
	var oldest string
	var expiry time.Time
	for key, entry := range c.entries {
		if oldest == "" || entry.expires.Before(expiry) {
			oldest, expiry = key, entry.expires
		}
	}
	if oldest != "" {
		c.bytes -= len(c.entries[oldest].data)
		delete(c.entries, oldest)
	}
}
