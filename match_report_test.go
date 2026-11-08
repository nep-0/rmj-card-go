package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/nep-0/rmj"
)

type fakeMatchHistory struct {
	pages []rmj.MatchHistoryPage
	calls int
}

func (f *fakeMatchHistory) ListMatchHistory(context.Context, string, string, int) (rmj.MatchHistoryPage, error) {
	page := f.pages[min(f.calls, len(f.pages)-1)]
	f.calls++
	return page, nil
}
func TestMatchReporterBaselineAndFreshDelivery(t *testing.T) {
	registry := newMatchReportRegistry(filepath.Join(t.TempDir(), "reports.json"))
	added, err := registry.Add("group", "club", "admin")
	if err != nil || !added {
		t.Fatalf("Add() = (%t, %v), want (true, nil)", added, err)
	}
	history := &fakeMatchHistory{pages: []rmj.MatchHistoryPage{{Records: []rmj.MatchRecord{{ID: "old"}}}, {Records: []rmj.MatchRecord{{ID: "new"}, {ID: "old"}}}}}
	var sent []string
	reporter := matchReporter{history: history, registry: registry, send: func(_ context.Context, group, message string) error {
		if group != "group" {
			t.Errorf("sent to group %q", group)
		}
		sent = append(sent, message)
		return nil
	}}
	if err := reporter.Begin(t.Context(), "group"); err != nil {
		t.Fatal(err)
	}
	if err := reporter.Poll(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(sent) != 1 || sent[0] != "1.  0\n2.  0\n3.  0\n4.  0" {
		t.Errorf("sent reports = %q", sent)
	}
	if err := reporter.End("group"); err != nil {
		t.Fatal(err)
	}
	groups, err := registry.Running()
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 0 {
		t.Errorf("running after End = %v", groups)
	}
}

func TestMatchReportRegistryRetriesFailedLoad(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, nil, 0600); err != nil {
		t.Fatal(err)
	}
	registry := newMatchReportRegistry(filepath.Join(blocker, "reports.json"))
	if added, err := registry.Add("group", "club", "admin"); err == nil || added {
		t.Fatal("Add succeeded without reporting a blocked registry parent")
	}
	if err := os.Remove(blocker); err != nil {
		t.Fatal(err)
	}
	if added, err := registry.Add("group", "club", "admin"); err != nil || !added {
		t.Errorf("Add() after unblocking parent = (%t, %v), want (true, nil)", added, err)
	}
}
