package main

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/nep-0/rmj"
)

type countingStatistics struct {
	fakeStatistics
	calls atomic.Int32
}

func (s *countingStatistics) GetPlayerStatistics(ctx context.Context, name string) (rmj.PlayerStats, error) {
	s.calls.Add(1)
	return s.fakeStatistics.GetPlayerStatistics(ctx, name)
}
func TestPNGCacheSkipsRepeatUpstreamLookup(t *testing.T) {
	stats := &countingStatistics{fakeStatistics: fakeStatistics{player: rmj.PlayerStats{Name: "Cache", Grade: 11}}}
	service := newCardService(stats)
	first, err := service.cachedPNG(t.Context(), "Cache", "modern")
	if err != nil {
		t.Fatal(err)
	}
	calls := stats.calls.Load()
	second, err := service.cachedPNG(t.Context(), "Cache", "modern")
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Error("cache hit changed PNG bytes")
	}
	if stats.calls.Load() != calls {
		t.Errorf("cached PNG triggered upstream lookup: before=%d after=%d", calls, stats.calls.Load())
	}
}
