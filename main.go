package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/nep-0/rmj"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	host := env("HOST", "")
	port := env("PORT", "3000")
	formula := rmj.NewFormulaClient(&http.Client{Timeout: 30 * time.Second})
	service := newCardService(formula)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/player-cards/", service.cardHandler)
	mux.HandleFunc("GET /api/player-stats/", service.statsHandler)
	mux.HandleFunc("GET /api/player-opponents/", service.opponentsHandler)
	metricsMux := http.NewServeMux()
	if envEnabled("TELEMETRY_ENABLED", false) {
		state, metricsHandler, err := newTelemetry(ctx)
		if err != nil {
			log.Fatal(fmt.Errorf("initialize OpenTelemetry: %w", err))
		}
		telemetryState = state
		metricsMux.Handle("/metrics", metricsHandler)
		metricsMux.Handle("/", instrumentRequests(mux))
	} else {
		metricsMux.Handle("/", mux)
	}

	server := &http.Server{Addr: net.JoinHostPort(host, port), Handler: metricsMux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if telemetryState != nil {
			if err := telemetryState.traces.Shutdown(shutdownCtx); err != nil {
				slog.Error("shutdown OpenTelemetry", "error", err)
			}
		}
		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error("shutdown HTTP server", "error", err)
		}
	}()

	if strings.EqualFold(os.Getenv("QQ_BOT_ENABLED"), "true") {
		if err := startQQBot(ctx, service); err != nil {
			log.Fatal(err)
		}
	}
	slog.Info("starting RMJ card service", "address", server.Addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(fmt.Errorf("serve HTTP: %w", err))
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func disabled(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}
func envEnabled(key string, fallback bool) bool {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes":
		return true
	case "0", "false", "no":
		return false
	default:
		return fallback
	}
}

func envInt(key string, fallback, minimum int) int {
	value, err := strconv.Atoi(env(key, strconv.Itoa(fallback)))
	if err != nil || value < minimum {
		return fallback
	}
	return value
}
