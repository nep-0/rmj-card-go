package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestPanelGroupsPersistAndIsolateList(t *testing.T) {
	file := filepath.Join(t.TempDir(), "groups.json")
	if err := os.WriteFile(file, []byte(`[" b ","a","a",""]`), 0600); err != nil {
		t.Fatal(err)
	}
	registry := newPanelGroupRegistry(file)
	groups, err := registry.List()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(groups, []string{"a", "b"}) {
		t.Errorf("loaded groups = %v, want [a b]", groups)
	}
	groups[0] = "changed"
	if added, err := registry.Add(" c "); err != nil || !added {
		t.Fatalf("Add(c) = (%t, %v)", added, err)
	}
	if added, err := registry.Add("c"); err != nil || added {
		t.Errorf("duplicate Add(c) = (%t, %v)", added, err)
	}
	restored, err := newPanelGroupRegistry(file).List()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(restored, []string{"a", "b", "c"}) {
		t.Errorf("restored groups = %v, want [a b c]", restored)
	}
}

func TestPanelGroupsRollBackFailedWrite(t *testing.T) {
	file := filepath.Join(t.TempDir(), "groups.json")
	registry := newPanelGroupRegistry(file)
	if err := registry.Load(); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(file, 0700); err != nil {
		t.Fatal(err)
	}
	if added, err := registry.Add("group"); err == nil || added {
		t.Fatal("Add succeeded with a directory at the file path")
	}
	groups, err := registry.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 0 {
		t.Errorf("failed write retained groups %v", groups)
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if added, err := registry.Add("group"); err != nil || !added {
		t.Errorf("retry Add = (%t, %v), want (true, nil)", added, err)
	}
}
