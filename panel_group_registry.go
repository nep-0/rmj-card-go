package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

type panelGroupRegistry struct {
	mu       sync.Mutex
	file     string
	loaded   bool
	groupIDs []string
}

func newPanelGroupRegistry(file string) *panelGroupRegistry {
	return &panelGroupRegistry{file: file}
}

func (r *panelGroupRegistry) Load() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.loadLocked()
}

func (r *panelGroupRegistry) List() ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.loadLocked(); err != nil {
		return nil, err
	}
	return slices.Clone(r.groupIDs), nil
}

func (r *panelGroupRegistry) Add(groupID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.loadLocked(); err != nil {
		return false, err
	}
	groupID = strings.TrimSpace(groupID)
	if groupID == "" || slices.Contains(r.groupIDs, groupID) {
		return false, nil
	}
	r.groupIDs = append(r.groupIDs, groupID)
	slices.Sort(r.groupIDs)
	if err := r.persistLocked(); err != nil {
		r.groupIDs = slices.Delete(r.groupIDs, slices.Index(r.groupIDs, groupID), slices.Index(r.groupIDs, groupID)+1)
		return false, err
	}
	return true, nil
}

func (r *panelGroupRegistry) loadLocked() error {
	if r.loaded {
		return nil
	}
	data, err := os.ReadFile(r.file)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read QQ panel groups: %w", err)
	}
	if err == nil {
		var groupIDs []string
		if err := json.Unmarshal(data, &groupIDs); err != nil {
			return fmt.Errorf("decode QQ panel groups: %w", err)
		}
		for _, id := range groupIDs {
			id = strings.TrimSpace(id)
			if id != "" && !slices.Contains(r.groupIDs, id) {
				r.groupIDs = append(r.groupIDs, id)
			}
		}
		slices.Sort(r.groupIDs)
	}
	r.loaded = true
	return nil
}

func (r *panelGroupRegistry) persistLocked() error {
	if err := os.MkdirAll(filepath.Dir(r.file), 0755); err != nil {
		return fmt.Errorf("create QQ panel groups directory: %w", err)
	}
	data, err := json.MarshalIndent(r.groupIDs, "", "  ")
	if err != nil {
		return fmt.Errorf("encode QQ panel groups: %w", err)
	}
	if err := os.WriteFile(r.file, append(data, '\n'), 0600); err != nil {
		return fmt.Errorf("write QQ panel groups: %w", err)
	}
	return nil
}
