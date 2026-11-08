package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

var qqIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

type qqNameCache struct {
	mu     sync.RWMutex
	file   string
	loaded bool
	values map[string]string
}

func newQQNameCache(file string) *qqNameCache {
	return &qqNameCache{file: file, values: make(map[string]string)}
}

func (c *qqNameCache) Load() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.loadLocked()
}

func (c *qqNameCache) Name(qq string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	name, ok := c.values[strings.TrimSpace(qq)]
	return name, ok
}

func (c *qqNameCache) Remember(qq, name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.loadLocked(); err != nil {
		return err
	}
	qq, name = strings.TrimSpace(qq), strings.TrimSpace(name)
	if !qqIdentifierPattern.MatchString(qq) || name == "" {
		return nil
	}
	previous, existed := c.values[qq]
	c.values[qq] = name
	if err := c.persistLocked(); err != nil {
		if existed {
			c.values[qq] = previous
		} else {
			delete(c.values, qq)
		}
		return err
	}
	return nil
}

func (c *qqNameCache) loadLocked() error {
	if c.loaded {
		return nil
	}
	data, err := os.ReadFile(c.file)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read QQ name cache: %w", err)
	}
	if err == nil {
		var values map[string]json.RawMessage
		if err := json.Unmarshal(data, &values); err != nil {
			return fmt.Errorf("decode QQ name cache: %w", err)
		}
		for qq, rawName := range values {
			var name string
			if !qqIdentifierPattern.MatchString(qq) || json.Unmarshal(rawName, &name) != nil || strings.TrimSpace(name) == "" {
				continue
			}
			c.values[qq] = name
		}
	}
	c.loaded = true
	return nil
}

func (c *qqNameCache) persistLocked() error {
	if err := os.MkdirAll(filepath.Dir(c.file), 0755); err != nil {
		return fmt.Errorf("create QQ name cache directory: %w", err)
	}
	keys := make([]string, 0, len(c.values))
	for qq := range c.values {
		keys = append(keys, qq)
	}
	sort.Strings(keys)
	data := make(map[string]string, len(c.values))
	for _, qq := range keys {
		data[qq] = c.values[qq]
	}
	encoded, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("encode QQ name cache: %w", err)
	}
	if err := os.WriteFile(c.file, append(encoded, '\n'), 0600); err != nil {
		return fmt.Errorf("write QQ name cache: %w", err)
	}
	return nil
}
