package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/oauth2"
)

const qqPanelAPIBase = "https://api.sgroup.qq.com"

type qqPanelClient struct {
	appID       string
	apiBase     string
	tokenSource oauth2.TokenSource
	httpClient  *http.Client
}

type qqPanelItem struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Desc      string `json:"desc,omitempty"`
	OnlyAdmin bool   `json:"only_admin,omitempty"`
	Link      string `json:"link,omitempty"`
}

type qqCommandPanel struct {
	Items   []qqPanelItem `json:"items"`
	Remark  string        `json:"remark,omitempty"`
	Version int           `json:"version,omitempty"`
}

type qqPanelRecord struct {
	PanelID    string         `json:"panel_id"`
	TargetType string         `json:"target_type"`
	Panel      qqCommandPanel `json:"panel"`
}

type qqPanelListResponse struct {
	Records []qqPanelRecord `json:"records"`
}

type qqPanelDetailResponse struct {
	Panel struct {
		Version int `json:"version"`
	} `json:"panel"`
}

func newQQPanelClient(appID string, tokenSource oauth2.TokenSource) *qqPanelClient {
	return &qqPanelClient{appID: appID, apiBase: qqPanelAPIBase, tokenSource: tokenSource, httpClient: &http.Client{Timeout: 30 * time.Second}}
}

func (c *qqPanelClient) Synchronize(ctx context.Context, groupIDs []string) error {
	if len(groupIDs) == 0 {
		return nil
	}
	panel := qqCommandPanel{
		Remark: "RMJ Card 群指令",
		Items: []qqPanelItem{
			{Name: "/help", Type: "command", Desc: "查看帮助"},
			{Name: "/bind 玩家名", Type: "command", Desc: "绑定 QQ 号与玩家名"},
			{Name: "/card [玩家名]", Type: "command", Desc: "生成玩家卡片"},
			{Name: "/stats [玩家名]", Type: "command", Desc: "查询玩家数据"},
			{Name: "/good [玩家名]", Type: "command", Desc: "查询好人榜"},
			{Name: "/bad [玩家名]", Type: "command", Desc: "查询仇人榜"},
		},
	}
	var panels qqPanelListResponse
	if err := c.request(ctx, http.MethodGet, "/v2/panels?scope=group&limit=100", nil, &panels); err != nil {
		return err
	}
	for _, current := range panels.Records {
		if current.TargetType != "specific" || current.Panel.Remark != panel.Remark {
			continue
		}
		id := url.PathEscape(current.PanelID)
		var detail qqPanelDetailResponse
		if err := c.request(ctx, http.MethodGet, "/v2/panels/"+id, nil, &detail); err != nil {
			return err
		}
		panel.Version = detail.Panel.Version
		if err := c.request(ctx, http.MethodPut, "/v2/panels/"+id, struct {
			Panel qqCommandPanel `json:"panel"`
		}{Panel: panel}, nil); err != nil {
			return err
		}
		return c.request(ctx, http.MethodPut, "/v2/panels/"+id+"/target", struct {
			Op           string   `json:"op"`
			GroupOpenIDs []string `json:"group_openids"`
		}{Op: "add", GroupOpenIDs: groupIDs}, nil)
	}
	return c.request(ctx, http.MethodPost, "/v2/panels", struct {
		Scope        string         `json:"scope"`
		TargetType   string         `json:"target_type"`
		GroupOpenIDs []string       `json:"group_openids"`
		Panel        qqCommandPanel `json:"panel"`
	}{Scope: "group", TargetType: "specific", GroupOpenIDs: groupIDs, Panel: panel}, nil)
}

func (c *qqPanelClient) request(ctx context.Context, method, path string, body, result any) error {
	token, err := c.tokenSource.Token()
	if err != nil {
		return fmt.Errorf("get QQ panel access token: %w", err)
	}
	var requestBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode QQ panel request: %w", err)
		}
		requestBody = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.apiBase+path, requestBody)
	if err != nil {
		return fmt.Errorf("create QQ panel request: %w", err)
	}
	req.Header.Set("Authorization", "QQBot "+token.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Union-Appid", c.appID)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send QQ panel request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiError struct {
			Code    json.RawMessage `json:"code"`
			Message string          `json:"message"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&apiError)
		return fmt.Errorf("QQ panel API returned HTTP %d (code %s): %s", resp.StatusCode, apiError.Code, apiError.Message)
	}
	if result == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("decode QQ panel response: %w", err)
	}
	return nil
}
