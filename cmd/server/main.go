package main

import (
	"context"
	"log"

	"messaging-app/config"
	"messaging-app/internal/server"
)

func main() {
	cfg := config.LoadConfig()
	metrics := config.GetMetrics()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	app, err := server.NewApplication(ctx, cfg, metrics)
	if err != nil {
		log.Fatalf("Failed to initialize application: %v", err)
	}

	if err := app.Run(); err != nil {
		log.Fatalf("Application stopped with error: %v", err)
	}
}
