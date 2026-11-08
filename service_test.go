package main

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nep-0/rmj"
)

type fakeStatistics struct{ player rmj.PlayerStats }

func (f fakeStatistics) GetPlayerStatistics(context.Context, string) (rmj.PlayerStats, error) {
	return f.player, nil
}
func (fakeStatistics) GetOpponentStatistics(context.Context, string, int, int) (rmj.OpponentStatisticsPage, error) {
	return rmj.OpponentStatisticsPage{}, nil
}
func (fakeStatistics) GetPlayerMatchHistory(context.Context, string, int, int) (rmj.PlayerMatchHistoryPage, error) {
	return rmj.PlayerMatchHistoryPage{}, nil
}

func TestServiceSmoke(t *testing.T) {
	player := rmj.PlayerStats{Name: "Smoke", Grade: 11, Rating: 1553, AveragePoints: 251.48, HighestPoints: 747, TotalGames: 10, FirstPlaces: 3, SecondPlaces: 3, ThirdPlaces: 2, FourthPlaces: 2, PromotionGameCount: 10, PromotionPositionSum: 25, PromotionAveragePlace: 2.5, RecentPositions: []int{1, 2, 4, 3, 2}, RecentPoints: []int{412, 381, 57, 183, 291}}
	service := newCardService(fakeStatistics{player: player})
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/player-cards/", service.cardHandler)
	server := httptest.NewServer(mux)
	defer server.Close()

	response, err := http.Get(server.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	var health map[string]string
	if err := json.NewDecoder(response.Body).Decode(&health); err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || health["status"] != "ok" {
		t.Fatalf("health = %v, status %d", health, response.StatusCode)
	}

	for _, style := range []string{"modern", "QH"} {
		response, err := http.Get(server.URL + "/api/player-cards/Smoke.png?style=" + style)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "image/png" {
			t.Fatalf("%s response status/content type = %d/%q", style, response.StatusCode, response.Header.Get("Content-Type"))
		}
		image, err := png.Decode(response.Body)
		_ = response.Body.Close()
		if err != nil {
			t.Fatalf("decode %s PNG: %v", style, err)
		}
		if image.Bounds().Dx() != cardWidth || image.Bounds().Dy() != cardHeight {
			t.Fatalf("%s PNG bounds = %v", style, image.Bounds())
		}
	}
}

func TestMatchHistoryOrder(t *testing.T) {
	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	records := []rmj.PlayerMatchRecord{
		{PlayedAt: older, First: rmj.Standing{Name: "Player", Points: 400}},
		{PlayedAt: older.Add(time.Hour), Third: rmj.Standing{Name: "Player", Points: 200}},
		{PlayedAt: older.Add(2 * time.Hour), Second: rmj.Standing{Name: "Someone else"}},
	}
	positions, points := matchHistory("Player", records)
	if !slices.Equal(positions, []int{3, 1}) || !slices.Equal(points, []int{200, 400}) {
		t.Errorf("matchHistory = %v, %v; want [3 1], [200 400]", positions, points)
	}
	if !records[0].PlayedAt.Equal(older) {
		t.Error("matchHistory changed caller record ordering")
	}
}

func TestPromotionProjectionBounds(t *testing.T) {
	player := rmj.PlayerStats{Grade: 11, PromotionPositionSum: 75, PromotionAveragePlace: 2.5, RecentPositions: make([]int, 30)}
	for i := range player.RecentPositions {
		player.RecentPositions[i] = 3
	}
	var chart strings.Builder
	renderTrend(&chart, player)
	decoder := xml.NewDecoder(strings.NewReader("<svg>" + chart.String() + "</svg>"))
	projections := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "polyline" {
			continue
		}
		attrs := make(map[string]string)
		for _, attr := range start.Attr {
			attrs[attr.Name.Local] = attr.Value
		}
		if attrs["stroke-dasharray"] == "" {
			continue
		}
		projections++
		points := strings.Fields(attrs["points"])
		if len(points) != 3 {
			t.Fatalf("forecast points = %v, want latest, first forecast, endpoint", points)
		}
		var forecastY string
		for i, point := range points {
			x, y, _ := strings.Cut(point, ",")
			value, err := strconv.ParseFloat(x, 64)
			if err != nil || value < 197 || value > cardWidth-100 {
				t.Errorf("projection x = %q, want inside chart", x)
			}
			if i == 1 {
				forecastY = y
			}
			if i == 2 && y != forecastY {
				t.Errorf("forecast endpoint y = %s, want horizontal at %s", y, forecastY)
			}
		}
	}
	if projections != 2 {
		t.Errorf("projection count = %d, want 2", projections)
	}
}
