package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/acme/outbound-call-campaign/internal/api"
	"github.com/acme/outbound-call-campaign/internal/api/handlers"
	"github.com/acme/outbound-call-campaign/internal/app"
	"github.com/acme/outbound-call-campaign/internal/telemetry"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	configPath := flag.String("config", getEnv("CONFIG_FILE", "configs/config.yaml"), "path to configuration file")
	flag.Parse()

	container, err := app.Build(ctx, *configPath)
	if err != nil {
		log.Fatalf("failed to bootstrap application: %v", err)
	}
	defer container.Close(context.Background())

	shutdown, err := telemetry.Setup(ctx, container.Config.Telemetry, container.Config.App.Name+"-api")
	if err != nil {
		log.Fatalf("failed to initialize telemetry: %v", err)
	}
	defer func() {
		_ = shutdown(context.Background())
	}()

	if err := container.EnsureTopics(ctx); err != nil {
		log.Fatalf("failed to ensure kafka topics: %v", err)
	}

	handlerSet := handlers.NewHandlerSet(container)
	server := api.NewServer(container, handlerSet)

	if err := server.Start(ctx); err != nil {
		log.Fatalf("server terminated: %v", err)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
