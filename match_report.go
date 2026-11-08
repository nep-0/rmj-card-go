package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nep-0/rmj"
)

type groupReport struct {
	ClubID       string   `json:"clubId"`
	Admins       []string `json:"admins"`
	Running      bool     `json:"running"`
	LastMatchIDs []string `json:"lastMatchIds"`
}
type matchReportRegistry struct {
	mu     sync.Mutex
	file   string
	loaded bool
	groups map[string]groupReport
}

func newMatchReportRegistry(file string) *matchReportRegistry {
	return &matchReportRegistry{file: file, groups: make(map[string]groupReport)}
}
func (r *matchReportRegistry) Load() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.loadLocked()
}
func (r *matchReportRegistry) loadLocked() error {
	if r.loaded {
		return nil
	}
	if dir := filepath.Dir(r.file); dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create match report directory: %w", err)
		}
	}
	data, err := os.ReadFile(r.file)
	if os.IsNotExist(err) {
		r.loaded = true
		return nil
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, &r.groups); err != nil {
		return fmt.Errorf("decode match report registry: %w", err)
	}
	if r.groups == nil {
		r.groups = make(map[string]groupReport)
	}
	r.loaded = true
	return nil
}
func (r *matchReportRegistry) persistLocked() error {
	if err := os.MkdirAll(filepath.Dir(r.file), 0755); err != nil {
		return fmt.Errorf("create match report directory: %w", err)
	}
	data, err := json.MarshalIndent(r.groups, "", "  ")
	if err != nil {
		return fmt.Errorf("encode match report registry: %w", err)
	}
	tmp := r.file + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0600); err != nil {
		return fmt.Errorf("write match report registry: %w", err)
	}
	if err := os.Rename(tmp, r.file); err != nil {
		if removeErr := os.Remove(tmp); removeErr != nil && !os.IsNotExist(removeErr) {
			return errors.Join(fmt.Errorf("replace match report registry: %w", err), fmt.Errorf("remove temporary registry: %w", removeErr))
		}
		return fmt.Errorf("replace match report registry: %w", err)
	}
	return nil
}
func (r *matchReportRegistry) Add(groupID, clubID, adminID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.loadLocked(); err != nil {
		return false, err
	}
	groupID, clubID, adminID = strings.TrimSpace(groupID), strings.TrimSpace(clubID), strings.TrimSpace(adminID)
	if groupID == "" || clubID == "" || adminID == "" || r.groups[groupID].ClubID != "" {
		return false, nil
	}
	r.groups[groupID] = groupReport{ClubID: clubID, Admins: []string{adminID}}
	if err := r.persistLocked(); err != nil {
		return false, err
	}
	return true, nil
}
func (r *matchReportRegistry) Get(groupID string) (groupReport, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.loadLocked() != nil {
		return groupReport{}, false
	}
	state, ok := r.groups[groupID]
	state.Admins = slices.Clone(state.Admins)
	state.LastMatchIDs = slices.Clone(state.LastMatchIDs)
	return state, ok
}
func (r *matchReportRegistry) IsAdmin(groupID, user string) bool {
	state, ok := r.Get(groupID)
	return ok && slices.Contains(state.Admins, user)
}
func (r *matchReportRegistry) AddAdmin(groupID, user string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.loadLocked(); err != nil {
		return false, err
	}
	state, ok := r.groups[groupID]
	user = strings.TrimSpace(user)
	if !ok || user == "" || slices.Contains(state.Admins, user) {
		return false, nil
	}
	state.Admins = append(state.Admins, user)
	r.groups[groupID] = state
	if err := r.persistLocked(); err != nil {
		return false, err
	}
	return true, nil
}
func (r *matchReportRegistry) SetRunning(groupID string, running bool, baseline []rmj.MatchRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.loadLocked(); err != nil {
		return err
	}
	state, ok := r.groups[groupID]
	if !ok {
		return fmt.Errorf("本群尚未配置对局报告，请先使用 /add 场所ID")
	}
	state.Running = running
	if baseline != nil {
		state.LastMatchIDs = recentMatchIDs(baseline)
	}
	r.groups[groupID] = state
	return r.persistLocked()
}
func (r *matchReportRegistry) Running() ([]struct {
	GroupID string
	State   groupReport
}, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.loadLocked(); err != nil {
		return nil, err
	}
	var groups []struct {
		GroupID string
		State   groupReport
	}
	for id, state := range r.groups {
		if state.Running {
			groups = append(groups, struct {
				GroupID string
				State   groupReport
			}{id, state})
		}
	}
	return groups, nil
}
func (r *matchReportRegistry) Update(groupID string, records []rmj.MatchRecord) ([]rmj.MatchRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.loadLocked(); err != nil {
		return nil, err
	}
	state, ok := r.groups[groupID]
	if !ok {
		return nil, fmt.Errorf("match report group %q not configured", groupID)
	}
	previous := make(map[string]bool, len(state.LastMatchIDs))
	for _, id := range state.LastMatchIDs {
		previous[id] = true
	}
	fresh := make([]rmj.MatchRecord, 0)
	for _, record := range records {
		id := record.ID
		if id == "" {
			id = record.RecordID
		}
		if id != "" && !previous[id] {
			fresh = append(fresh, record)
		}
	}
	state.LastMatchIDs = recentMatchIDs(records)
	r.groups[groupID] = state
	if err := r.persistLocked(); err != nil {
		return nil, err
	}
	slices.Reverse(fresh)
	return fresh, nil
}
func recentMatchIDs(records []rmj.MatchRecord) []string {
	ids := make([]string, 0, 12)
	for _, record := range records {
		id := record.ID
		if id == "" {
			id = record.RecordID
		}
		if id != "" {
			ids = append(ids, id)
			if len(ids) == 12 {
				break
			}
		}
	}
	return ids
}

type matchReporter struct {
	history  rmj.MatchHistory
	registry *matchReportRegistry
	send     func(context.Context, string, string) error
	interval time.Duration
	polling  atomic.Bool
}

func (m *matchReporter) Start(ctx context.Context) {
	interval := m.interval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := m.Poll(ctx); err != nil {
					fmt.Printf("match report polling: %v\n", err)
				}
			}
		}
	}()
}
func (m *matchReporter) Begin(ctx context.Context, groupID string) error {
	state, ok := m.registry.Get(groupID)
	if !ok {
		return fmt.Errorf("本群尚未配置对局报告，请先使用 /add 场所ID")
	}
	page, err := m.history.ListMatchHistory(ctx, state.ClubID, "", 12)
	if err != nil {
		return err
	}
	return m.registry.SetRunning(groupID, true, page.Records)
}
func (m *matchReporter) End(groupID string) error { return m.registry.SetRunning(groupID, false, nil) }
func (m *matchReporter) Poll(ctx context.Context) error {
	if !m.polling.CompareAndSwap(false, true) {
		return nil
	}
	defer m.polling.Store(false)
	groups, err := m.registry.Running()
	if err != nil {
		return err
	}
	var errs []error
	for _, group := range groups {
		page, err := m.history.ListMatchHistory(ctx, group.State.ClubID, "", 12)
		if err != nil {
			errs = append(errs, fmt.Errorf("fetch club %s: %w", group.State.ClubID, err))
			continue
		}
		fresh, err := m.registry.Update(group.GroupID, page.Records)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for _, record := range fresh {
			if err := m.send(ctx, group.GroupID, renderMatchReport(record)); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}
func renderMatchReport(record rmj.MatchRecord) string {
	return fmt.Sprintf("1. %s %d\n2. %s %d\n3. %s %d\n4. %s %d", record.First.Name, record.First.Points, record.Second.Name, record.Second.Points, record.Third.Name, record.Third.Points, record.Fourth.Name, record.Fourth.Points)
}
