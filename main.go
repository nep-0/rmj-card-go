package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/nep-0/rmj"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

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

	server := &http.Server{Addr: ":" + port, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
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
