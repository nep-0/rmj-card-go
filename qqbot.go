package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/nep-0/rmj"
	"github.com/tencent-connect/botgo"
	"github.com/tencent-connect/botgo/dto"
	"github.com/tencent-connect/botgo/event"
	"github.com/tencent-connect/botgo/openapi"
	"github.com/tencent-connect/botgo/token"
)

type qqBot struct {
	api      openapi.OpenAPI
	cards    *cardService
	mu       sync.RWMutex
	players  map[string]string
	reports  *matchReportRegistry
	reporter *matchReporter
}

func startQQBot(ctx context.Context, cards *cardService) error {
	appid, secret := os.Getenv("QQ_BOT_APPID"), os.Getenv("QQ_BOT_SECRET")
	if appid == "" || secret == "" {
		return fmt.Errorf("QQ_BOT_APPID and QQ_BOT_SECRET are required")
	}
	credentials := &token.QQBotCredentials{AppID: appid, AppSecret: secret}
	tokenSource := token.NewQQBotTokenSource(credentials)
	if err := token.StartRefreshAccessToken(ctx, tokenSource); err != nil {
		return fmt.Errorf("start QQ token refresh: %w", err)
	}
	api := botgo.NewOpenAPI(appid, tokenSource).WithTimeout(30 * time.Second)
	reports := newMatchReportRegistry(env("QQ_BOT_MATCH_REPORTS_FILE", "data/qq-match-reports.json"))
	interval := 30 * time.Second
	if ms, err := time.ParseDuration(env("QQ_BOT_MATCH_REPORT_INTERVAL_MS", "30000") + "ms"); err == nil {
		interval = ms
	}
	bot := &qqBot{api: api, cards: cards, players: make(map[string]string), reports: reports}
	bot.reporter = &matchReporter{history: rmj.NewMatchHistoryClient(&http.Client{Timeout: 30 * time.Second}), registry: reports, interval: interval, send: func(ctx context.Context, groupID, message string) error {
		_, err := api.PostGroupMessage(ctx, groupID, &dto.MessageToCreate{Content: message})
		return err
	}}
	bot.reporter.Start(ctx)
	intent := event.RegisterHandlers(bot.groupHandler())
	websocket, err := api.WS(ctx, nil, "")
	if err != nil {
		return fmt.Errorf("connect QQ bot gateway: %w", err)
	}
	go func() {
		if err := botgo.NewSessionManager().Start(websocket, tokenSource, &intent); err != nil && ctx.Err() == nil {
			slog.Error("QQ bot websocket stopped", "error", err)
		}
	}()
	return nil
}

func (b *qqBot) groupHandler() event.GroupATMessageEventHandler {
	return func(_ *dto.WSPayload, data *dto.WSGroupATMessageData) error {
		content := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(data.Content), "@"))
		prefix := env("QQ_BOT_COMMAND_PREFIX", "/")
		if !strings.HasPrefix(content, prefix) {
			return nil
		}
		parts := strings.Fields(strings.TrimPrefix(content, prefix))
		if len(parts) == 0 {
			return nil
		}
		command, arg := parts[0], strings.Join(parts[1:], " ")
		user := ""
		if data.Author != nil {
			user = data.Author.ID
		}
		ctx := context.Background()
		if command == "help" {
			return b.reply(data, "可用指令：\n/add 场所ID - 配置本群对局报告\n/start - 开始对局报告\n/end - 停止对局报告\n/admin 用户ID - 添加报告管理员\n/bind 玩家名 - 绑定玩家名\n/card [玩家名] - 生成玩家卡片\n/stats [玩家名] - 查询玩家数据\n/good [玩家名] - 查询好人榜\n/bad [玩家名] - 查询仇人榜")
		}
		if command == "add" {
			if arg == "" {
				return b.reply(data, "用法：/add 场所ID")
			}
			if user == "" {
				return b.reply(data, "无法识别操作者。")
			}
			added, err := b.reports.Add(data.GroupID, arg, user)
			if err != nil {
				return b.reply(data, "对局报告配置写入失败："+err.Error())
			}
			if !added {
				return b.reply(data, "本群已配置对局报告。")
			}
			return b.reply(data, "已配置雀庄 "+arg+"；你已成为管理员。")
		}
		if command == "start" || command == "end" || command == "admin" {
			if !b.reports.IsAdmin(data.GroupID, user) {
				return nil
			}
			switch command {
			case "start":
				if err := b.reporter.Begin(ctx, data.GroupID); err != nil {
					return b.reply(data, err.Error())
				}
				return b.reply(data, "对局报告已启动。")
			case "end":
				if err := b.reporter.End(data.GroupID); err != nil {
					return b.reply(data, err.Error())
				}
				return b.reply(data, "对局报告已停止。")
			case "admin":
				if arg == "" {
					return b.reply(data, "用法：/admin 用户ID")
				}
				added, err := b.reports.AddAdmin(data.GroupID, arg)
				if err != nil {
					return b.reply(data, "管理员配置写入失败："+err.Error())
				}
				if !added {
					return b.reply(data, "用户已是管理员，或本群未配置对局报告。")
				}
				return b.reply(data, "已添加管理员。")
			}
		}
		if command == "bind" {
			if arg == "" {
				return b.reply(data, "用法：/bind 玩家名")
			}
			b.mu.Lock()
			b.players[user] = arg
			b.mu.Unlock()
			return b.reply(data, fmt.Sprintf("已将 QQ %s 绑定到玩家「%s」。", user, arg))
		}
		name := arg
		if name == "" {
			b.mu.RLock()
			name = b.players[user]
			b.mu.RUnlock()
		}
		if name == "" {
			return b.reply(data, fmt.Sprintf("未找到你的玩家名称，请使用 /%s 玩家名 查询。", command))
		}
		switch command {
		case "card":
			base := strings.TrimRight(os.Getenv("QQ_BOT_PUBLIC_BASE_URL"), "/")
			if base == "" {
				return b.reply(data, "QQ_BOT_PUBLIC_BASE_URL 未配置，无法发送卡片图片。")
			}
			style := env("QQ_BOT_STYLE", "modern")
			if style != "modern" && style != "QH" {
				return b.reply(data, "QQ_BOT_STYLE must be modern or QH")
			}
			return b.reply(data, fmt.Sprintf("玩家卡片：%s/api/player-cards/%s.png?style=%s", base, url.PathEscape(name), url.QueryEscape(style)))
		case "stats":
			player, err := b.cards.player(ctx, name)
			if err != nil {
				return b.reply(data, err.Error())
			}
			return b.reply(data, fmt.Sprintf("%s 玩家统计\n段位 %s RATE %d\n总分 %.2f 平均点数 %.2f\n对局 %d，一位 %d 二位 %d 三位 %d 四位 %d\n平均顺位 %.2f", player.Name, gradeName(player.Grade), player.Rating, player.TotalScore, player.AveragePoints, player.TotalGames, player.FirstPlaces, player.SecondPlaces, player.ThirdPlaces, player.FourthPlaces, averagePlace(player)))
		case "good", "bad":
			player, err := b.cards.player(ctx, name)
			if err != nil {
				return b.reply(data, err.Error())
			}
			page := 1
			if command == "good" {
				first, err := b.cards.stats.GetOpponentStatistics(ctx, player.Name, 1, 10)
				if err != nil {
					return b.reply(data, err.Error())
				}
				page = max(1, first.Pages-1)
				if first.Pages > 1 {
					previous, err := b.cards.stats.GetOpponentStatistics(ctx, player.Name, first.Pages-1, 10)
					if err != nil {
						return b.reply(data, err.Error())
					}
					latest, err := b.cards.stats.GetOpponentStatistics(ctx, player.Name, first.Pages, 10)
					if err != nil {
						return b.reply(data, err.Error())
					}
					records := append(previous.Records, latest.Records...)
					if len(records) > 10 {
						records = records[len(records)-10:]
					}
					return b.reply(data, renderOpponents(player.Name, "好人榜", "好人值", records))
				}
			}
			result, err := b.cards.stats.GetOpponentStatistics(ctx, player.Name, page, 10)
			if err != nil {
				return b.reply(data, err.Error())
			}
			label, valueLabel := "仇人榜", "仇人值"
			if command == "good" {
				label, valueLabel = "好人榜", "好人值"
			}
			return b.reply(data, renderOpponents(player.Name, label, valueLabel, result.Records))
		default:
			return nil
		}
	}
}

func (b *qqBot) reply(data *dto.WSGroupATMessageData, text string) error {
	_, err := b.api.PostGroupMessage(context.Background(), data.GroupID, &dto.MessageToCreate{Content: text, MsgID: data.ID})
	if err != nil {
		return fmt.Errorf("reply to QQ group message: %w", err)
	}
	return nil
}

func renderOpponents(name, label, valueLabel string, opponents []rmj.OpponentStats) string {
	lines := []string{name + " " + label, valueLabel + " 相遇 胜率 对手"}
	for _, opponent := range opponents {
		lines = append(lines, fmt.Sprintf("%3d  %4d  %6.2f%%  %s", opponent.Rivalry, opponent.Encounters, opponent.MyWinRate*100, opponent.OpponentName))
	}
	return strings.Join(lines, "\n")
}
