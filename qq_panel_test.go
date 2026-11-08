package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"golang.org/x/oauth2"
)

func TestPanelSynchronizeCreateAndUpdate(t *testing.T) {
	var created, updated, targeted bool
	groups := []string{"group-a", "group-b"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "QQBot access" {
			t.Errorf("authorization = %q, want QQBot access", got)
		}
		if got := r.Header.Get("X-Union-Appid"); got != "app" {
			t.Errorf("appid = %q, want app", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "GET /v2/panels":
			if created {
				_, _ = w.Write([]byte(`{"records":[{"panel_id":"existing","target_type":"specific","panel":{"remark":"RMJ Card 群指令"}}]}`))
			} else {
				_, _ = w.Write([]byte(`{"records":[]}`))
			}
		case "POST /v2/panels":
			var body struct {
				Scope      string   `json:"scope"`
				TargetType string   `json:"target_type"`
				Groups     []string `json:"group_openids"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
				http.Error(w, "bad request", 400)
				return
			}
			if body.Scope != "group" || body.TargetType != "specific" || !slices.Equal(body.Groups, groups) {
				t.Errorf("create target = %+v", body)
			}
			created = true
			w.WriteHeader(http.StatusNoContent)
		case "GET /v2/panels/existing":
			_, _ = w.Write([]byte(`{"panel":{"version":7}}`))
		case "PUT /v2/panels/existing":
			var body struct {
				Panel struct {
					Version int `json:"version"`
				} `json:"panel"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
				http.Error(w, "bad request", 400)
				return
			}
			if body.Panel.Version != 7 {
				t.Errorf("update version = %d, want 7", body.Panel.Version)
			}
			updated = true
			w.WriteHeader(http.StatusNoContent)
		case "PUT /v2/panels/existing/target":
			var body struct {
				Op     string   `json:"op"`
				Groups []string `json:"group_openids"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
				http.Error(w, "bad request", 400)
				return
			}
			if !updated || body.Op != "add" || !slices.Equal(body.Groups, groups) {
				t.Errorf("target update = %+v, panel updated = %t", body, updated)
			}
			targeted = true
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := newQQPanelClient("app", oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "access", TokenType: "Bearer"}))
	client.apiBase, client.httpClient = server.URL, server.Client()
	if err := client.Synchronize(t.Context(), groups); err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("panel was not created")
	}
	if err := client.Synchronize(t.Context(), groups); err != nil {
		t.Fatal(err)
	}
	if !updated || !targeted {
		t.Errorf("updated = %t, targeted = %t, want both true", updated, targeted)
	}
}

func TestPanelSynchronizeStopsOnAPIError(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.Error(w, `{"code":42,"message":"denied"}`, http.StatusForbidden)
	}))
	defer server.Close()
	client := newQQPanelClient("app", oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "access"}))
	client.apiBase, client.httpClient = server.URL, server.Client()
	if err := client.Synchronize(t.Context(), []string{"group"}); err == nil {
		t.Error("Synchronize succeeded after API rejection")
	}
	if requests != 1 {
		t.Errorf("requests = %d, want 1", requests)
	}
	if err := client.Synchronize(t.Context(), nil); err != nil {
		t.Error(err)
	}
	if requests != 1 {
		t.Errorf("empty groups made a request: requests = %d", requests)
	}
}
