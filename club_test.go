package main

import (
	"github.com/nep-0/rmj"
	"testing"
	"time"
)

func TestLatestClub(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	records := []rmj.PlayerMatchRecord{
		{MahjongName: "Old", PlayedAt: now},
		{MahjongName: "  New  ", PlayedAt: now.Add(time.Hour)},
		{MahjongName: " ", PlayedAt: now.Add(2 * time.Hour)},
	}
	if got := latestClub(records); got != "New" {
		t.Errorf("latestClub = %q, want New", got)
	}
	if got := latestClub(nil); got != "" {
		t.Errorf("latestClub(nil) = %q, want empty", got)
	}
}
