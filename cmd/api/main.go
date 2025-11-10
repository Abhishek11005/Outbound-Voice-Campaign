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
)

func main() {
	log.Println("Starting API server...")

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	configPath := flag.String("config", getEnv("CONFIG_FILE", "configs/config.yaml"), "path to configuration file")
	flag.Parse()

	log.Printf("Using config file: %s", *configPath)

	log.Println("Building container...")
	container, err := app.Build(ctx, *configPath)
	if err != nil {
		log.Fatalf("failed to bootstrap application: %v", err)
	}
	defer container.Close(context.Background())
	log.Println("Container built successfully")

	log.Println("Creating handlers...")
	handlerSet := handlers.NewHandlerSet(container)
	log.Println("Handlers created successfully")

	log.Println("Creating server...")
	server := api.NewServer(container, handlerSet)
	log.Println("Server created successfully")

	log.Printf("Starting server on port %d...", container.Config.HTTP.Port)
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
