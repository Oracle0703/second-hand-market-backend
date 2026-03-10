package main

import (
	"log"

	"second-hand-market-backend/backend/internal/app"
)

func main() {
	cfg := app.LoadConfig()
	srv, err := app.NewServer(cfg)
	if err != nil {
		log.Fatalf("failed to init server: %v", err)
	}
	if err := srv.Run(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
