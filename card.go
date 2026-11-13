package main

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/nep-0/rmj"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type cardService struct {
	stats     rmj.PlayerStatistics
	images    *renderCache
	nameCache *qqNameCache
}

func newCardService(stats rmj.PlayerStatistics) *cardService {
	maxEntries := envInt("QQ_BOT_CARD_CACHE_MAX_ENTRIES", 512, 1)
	maxBytes := envInt("QQ_BOT_CARD_CACHE_MAX_BYTES", 67108864, 1)
	return &cardService{stats: stats, images: newRenderCache(maxEntries, maxBytes, 5*time.Minute)}
}

func (s *cardService) player(ctx context.Context, name string) (rmj.PlayerStats, error) {
	ctx, span := otel.Tracer("rmj-card").Start(ctx, "data.player_statistics")
	defer span.End()
	if strings.TrimSpace(name) == "" {
		return rmj.PlayerStats{}, fmt.Errorf("player name must not be empty")
	}
	player, err := s.stats.GetPlayerStatistics(ctx, name)
	if err != nil {
		span.RecordError(err)
		return rmj.PlayerStats{}, fmt.Errorf("fetch player statistics: %w", err)
	}
	if s.nameCache != nil {
		if err := s.nameCache.Remember(player.QQ, player.Name); err != nil {
			return rmj.PlayerStats{}, fmt.Errorf("remember QQ player name: %w", err)
		}
	}
	return player, nil
}

func (s *cardService) cardHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/player-cards/")
	name, format, ok := strings.Cut(path, ".")
	if !ok {
		http.NotFound(w, r)
		return
	}
	r.SetPathValue("name", name)
	r.SetPathValue("format", format)
	if format != "svg" && format != "png" {
		http.NotFound(w, r)
		return
	}
	if format == "svg" && disabled(env("DISABLE_SVG", "false")) {
		http.NotFound(w, r)
		return
	}
	style := r.URL.Query().Get("style")
	if style == "" {
		style = "modern"
	}
	if style != "modern" && style != "QH" {
		http.Error(w, `{"error":"Supported styles are modern and QH"}`, http.StatusNotFound)
		return
	}
	if format == "png" {
		image, err := s.cachedPNG(r.Context(), name, style)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s-%s-formula-card.png"`, name, style))
		w.Header().Set("Cache-Control", "public, max-age=300")
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(image)
		return
	}
	player, err := s.player(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	svg, err := s.cardSVG(r.Context(), player, style)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s-%s-formula-card.svg"`, player.Name, style))
	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	_, _ = w.Write([]byte(svg))
}

func (s *cardService) cardSVG(ctx context.Context, player rmj.PlayerStats, style string) (string, error) {
	ctx, span := otel.Tracer("rmj-card").Start(ctx, "card.compose", trace.WithAttributes(attribute.String("card.style", style)))
	defer span.End()
	historyCtx, historySpan := otel.Tracer("rmj-card").Start(ctx, "data.player_match_history")
	history, err := s.stats.GetPlayerMatchHistory(historyCtx, player.Name, 1, 50)
	if err != nil {
		historySpan.RecordError(err)
	}
	historySpan.End()
	club := ""
	if err == nil {
		if positions, points := matchHistory(player.Name, history.Records); len(positions) > 0 {
			player.RecentPositions, player.RecentPoints = positions, points
		}
		club = latestClub(history.Records)
	}
	svg := renderCard(player, style, club)
	if avatar, err := fetchAvatar(ctx, avatarClient, player.QQ); err == nil {
		svg = embedAvatar(svg, avatar)
	}
	return svg, nil
}

func (s *cardService) cachedPNG(ctx context.Context, name, style string) ([]byte, error) {
	ctx, span := otel.Tracer("rmj-card").Start(ctx, "card.render_png", trace.WithAttributes(attribute.String("card.style", style)))
	defer span.End()
	key := name + "\x00" + style
	return s.images.load(ctx, key, func(ctx context.Context) ([]byte, error) {
		player, err := s.player(ctx, name)
		if err != nil {
			return nil, err
		}
		svg, err := s.cardSVG(ctx, player, style)
		if err != nil {
			return nil, err
		}
		renderCtx, renderSpan := otel.Tracer("rmj-card").Start(ctx, "image.draw")
		data, err := renderPNG(renderCtx, svg)
		if err != nil {
			renderSpan.RecordError(err)
		}
		renderSpan.End()
		return data, err
	})
}

func (s *cardService) statsHandler(w http.ResponseWriter, r *http.Request) {
	r.SetPathValue("name", strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/player-stats/"), ".txt"))
	player, err := s.player(r.Context(), r.PathValue("name"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = fmt.Fprintf(w, "%s 玩家统计\n段位 %s  RATE %d\n总分 %.2f  平均点数 %.2f\n对局 %d  一位 %d  二位 %d  三位 %d  四位 %d\n一位率 %.2f%%  平均顺位 %.2f\n升段场次 %d  最大点数 %d", player.Name, gradeName(player.Grade), player.Rating, player.TotalScore, player.AveragePoints, player.TotalGames, player.FirstPlaces, player.SecondPlaces, player.ThirdPlaces, player.FourthPlaces, percentage(player.FirstPlaces, player.TotalGames), averagePlace(player), player.PromotionGameCount, player.HighestPoints)
}

func (s *cardService) opponentsHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/player-opponents/")
	rankingKind, name, ok := strings.Cut(path, "/")
	if !ok {
		http.NotFound(w, r)
		return
	}
	name = strings.TrimSuffix(name, ".txt")
	player, err := s.player(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if rankingKind != "good" && rankingKind != "bad" {
		http.NotFound(w, r)
		return
	}
	page := 1
	if rankingKind == "good" {
		firstPage, err := s.stats.GetOpponentStatistics(r.Context(), player.Name, 1, 10)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		last := max(1, firstPage.Pages)
		if last > 1 {
			penultimate, err := s.stats.GetOpponentStatistics(r.Context(), player.Name, last-1, 10)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
			latest, err := s.stats.GetOpponentStatistics(r.Context(), player.Name, last, 10)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
			records := append(penultimate.Records, latest.Records...)
			if len(records) > 10 {
				records = records[len(records)-10:]
			}
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write([]byte(renderOpponents(player.Name, "好人榜", "好人值", records)))
			return
		}
	}
	result, err := s.stats.GetOpponentStatistics(r.Context(), player.Name, page, 10)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	label, valueLabel := "仇人榜", "仇人值"
	if rankingKind == "good" {
		label, valueLabel = "好人榜", "好人值"
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(renderOpponents(player.Name, label, valueLabel, result.Records)))
}

func gradeName(number int) string {
	grade, ok := rmj.GradeByNumber(number)
	if ok && grade.Name != "" {
		return grade.Name
	}
	return "未知段位"
}

func averagePlace(player rmj.PlayerStats) float64 {
	if player.TotalGames == 0 {
		return 0
	}
	return float64(player.FirstPlaces+2*player.SecondPlaces+3*player.ThirdPlaces+4*player.FourthPlaces) / float64(player.TotalGames)
}

func percentage(count, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(count) * 100 / float64(total)
}

func escape(value string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;", "'", "&apos;").Replace(value)
}

func matchHistory(name string, records []rmj.PlayerMatchRecord) ([]int, []int) {
	records = slices.Clone(records)
	slices.SortFunc(records, func(a, b rmj.PlayerMatchRecord) int { return b.PlayedAt.Compare(a.PlayedAt) })
	var placements, points []int
	for _, record := range records {
		for place, standing := range [4]rmj.Standing{record.First, record.Second, record.Third, record.Fourth} {
			if standing.Name == name {
				placements = append(placements, place+1)
				points = append(points, standing.Points)
				break
			}
		}
	}
	return placements, points
}

func latestClub(records []rmj.PlayerMatchRecord) string {
	var latest rmj.PlayerMatchRecord
	for _, record := range records {
		if strings.TrimSpace(record.MahjongName) != "" && (latest.MahjongName == "" || record.PlayedAt.After(latest.PlayedAt)) {
			latest = record
		}
	}
	return strings.TrimSpace(latest.MahjongName)
}
