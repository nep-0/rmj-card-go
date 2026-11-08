package main

import (
	"context"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nep-0/rmj"
	"github.com/tencent-connect/botgo/dto"
	"github.com/tencent-connect/botgo/openapi"
	"github.com/tencent-connect/botgo/openapi/options"
)

type replyCapture struct {
	openapi.OpenAPI
	message *dto.MessageToCreate
}

func (c *replyCapture) PostGroupMessage(_ context.Context, _ string, message dto.APIMessage, _ ...options.Option) (*dto.Message, error) {
	c.message = message.(*dto.MessageToCreate)
	return &dto.Message{}, nil
}

func TestGroupCardRendersImageAndCommands(t *testing.T) {
	service := newCardService(fakeStatistics{player: rmj.PlayerStats{Name: "Smoke", Grade: 11}})
	server := httptest.NewServer(http.HandlerFunc(service.cardHandler))
	defer server.Close()
	t.Setenv("QQ_BOT_PUBLIC_BASE_URL", server.URL)
	t.Setenv("QQ_BOT_STYLE", "modern")
	t.Setenv("QQ_BOT_COMMAND_PREFIX", "/")
	capture := &replyCapture{}
	bot := &qqBot{api: capture, cards: service}
	if err := bot.groupHandler()(nil, &dto.WSGroupATMessageData{Content: "/card Smoke", ID: "incoming", GroupID: "group"}); err != nil {
		t.Fatal(err)
	}
	message := capture.message
	if message.MsgType != dto.MarkdownMsg || message.Markdown == nil {
		t.Fatalf("card reply is not markdown: %+v", message)
	}
	content := message.Markdown.Content
	start := strings.Index(content, "](")
	if !strings.HasPrefix(content, "![") || start < 0 || !strings.HasSuffix(content, ")") {
		t.Fatalf("card reply is not an inline image: %q", content)
	}
	imageURL := content[start+2 : len(content)-1]
	parsed, err := url.Parse(imageURL)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Query().Get("v") == "" {
		t.Error("image URL has no rendered-image version")
	}
	response, err := http.Get(imageURL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("image status = %d", response.StatusCode)
	}
	if _, err := png.Decode(response.Body); err != nil {
		t.Fatalf("inline image is not PNG: %v", err)
	}
	if message.MsgID != "incoming" {
		t.Errorf("reply msg_id = %q", message.MsgID)
	}
	if message.Keyboard == nil || message.Keyboard.Content == nil {
		t.Fatal("card has no keyboard")
	}
	buttons := message.Keyboard.Content.Rows[0].Buttons
	for i, command := range []string{"/card", "/good", "/bad"} {
		if i >= len(buttons) {
			t.Fatalf("missing command %s", command)
		}
		action := buttons[i].Action
		if action.Data != command || action.Type != 2 || !action.Enter || action.Permission.Type != 2 {
			t.Errorf("button action = %+v, want executable %s for all users", action, command)
		}
	}
}

func TestUnboundCommandOffersBindButton(t *testing.T) {
	t.Setenv("QQ_BOT_COMMAND_PREFIX", "/")
	capture := &replyCapture{}
	bot := &qqBot{api: capture, players: newQQNameCache(filepath.Join(t.TempDir(), "names.json"))}
	if err := bot.groupHandler()(nil, &dto.WSGroupATMessageData{Content: "/card"}); err != nil {
		t.Fatal(err)
	}
	message := capture.message
	if message.MsgType != dto.MarkdownMsg || message.Keyboard == nil {
		t.Fatal("unbound response has no markdown keyboard")
	}
	action := message.Keyboard.Content.Rows[0].Buttons[0].Action
	if action.Data != "/bind " || action.Type != 2 || !action.Enter {
		t.Errorf("bind action = %+v", action)
	}
}
