package main

import (
	"context"
	"testing"
)

func TestDisabled(t *testing.T) {
	for _, value := range []string{"1", "true", "TRUE", " yes ", "Yes"} {
		if !disabled(value) {
			t.Errorf("disabled(%q) = false, want true", value)
		}
	}
	for _, value := range []string{"", "false", "0", "no", " true-ish "} {
		if disabled(value) {
			t.Errorf("disabled(%q) = true, want false", value)
		}
	}
}

func TestEnvIntBounds(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  int
	}{
		{"2", 2}, {"0", 8}, {"-1", 8}, {"invalid", 8}, {"999999999999999999999999", 8},
	} {
		t.Run(tc.value, func(t *testing.T) {
			t.Setenv("RMJ_TEST_LIMIT", tc.value)
			if got := envInt("RMJ_TEST_LIMIT", 8, 1); got != tc.want {
				t.Errorf("envInt(%q) = %d, want %d", tc.value, got, tc.want)
			}
		})
	}
}

func TestEnvEnabledExplicitFalse(t *testing.T) {
	for _, value := range []string{"false", "0", " no "} {
		t.Setenv("RMJ_TEST_ENABLED", value)
		if envEnabled("RMJ_TEST_ENABLED", true) {
			t.Errorf("envEnabled(%q, true) = true, want false", value)
		}
	}
}

func TestConfiguredCacheLimits(t *testing.T) {
	for _, tc := range []struct{ name, entries, bytes string }{
		{"entry limit", "1", "100"},
		{"byte limit", "100", "3"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("QQ_BOT_CARD_CACHE_MAX_ENTRIES", tc.entries)
			t.Setenv("QQ_BOT_CARD_CACHE_MAX_BYTES", tc.bytes)
			cache := newCardService(nil).images
			for _, key := range []string{"first", "second"} {
				if _, err := cache.load(t.Context(), key, func(context.Context) ([]byte, error) { return []byte("png"), nil }); err != nil {
					t.Fatal(err)
				}
			}
			if _, ok := cache.get("first"); ok {
				t.Error("configured limit did not evict first image")
			}
			if data, ok := cache.get("second"); !ok || string(data) != "png" {
				t.Errorf("latest cached image = (%q, %t)", data, ok)
			}
		})
	}
}
