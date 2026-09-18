package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"second-hand-market-backend/backend/internal/app"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cfg := app.LoadConfig()
	srv, err := app.NewServer(cfg)
	if err != nil {
		log.Fatalf("failed to init server: %v", err)
	}
	if err := srv.RunContext(ctx); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
