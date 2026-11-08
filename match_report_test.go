package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
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

func TestQQNameCacheReadsAndWritesUpstreamFormat(t *testing.T) {
	file := filepath.Join(t.TempDir(), "qq-name-cache.json")
	if err := os.WriteFile(file, []byte("{\"12345\":\"Existing Name\"}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cache := newQQNameCache(file)
	if err := cache.Load(); err != nil {
		t.Fatal(err)
	}
	if name, ok := cache.Name("12345"); !ok || name != "Existing Name" {
		t.Fatalf("Name(12345) = (%q, %t), want (%q, true)", name, ok, "Existing Name")
	}
	if err := cache.Remember("67890", "New Name"); err != nil {
		t.Fatal(err)
	}
	var got map[string]string
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"12345": "Existing Name", "67890": "New Name"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("QQ name cache = %#v, want %#v", got, want)
	}
	reloaded := newQQNameCache(file)
	if err := reloaded.Load(); err != nil {
		t.Fatal(err)
	}
	if name, ok := reloaded.Name("67890"); !ok || name != "New Name" {
		t.Errorf("reloaded Name(67890) = (%q, %t), want (%q, true)", name, ok, "New Name")
	}
}

func TestMatchReportRegistryReadsUpstreamFormat(t *testing.T) {
	file := filepath.Join(t.TempDir(), "qq-match-reports.json")
	data := []byte(`{"group-1":{"clubId":"122","admins":["owner"],"running":true,"lastMatchIds":["match-1"]}}`)
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	registry := newMatchReportRegistry(file)
	if err := registry.Load(); err != nil {
		t.Fatal(err)
	}
	state, ok := registry.Get("group-1")
	if !ok || state.ClubID != "122" || !state.Running || !slices.Equal(state.Admins, []string{"owner"}) || !slices.Equal(state.LastMatchIDs, []string{"match-1"}) {
		t.Errorf("Get(group-1) = (%+v, %t), want upstream state", state, ok)
	}
}
