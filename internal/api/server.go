package api

import (
	"context"
	"fmt"
	"time"

	"github.com/gofiber/contrib/otelfiber"
	"github.com/gofiber/fiber/v2"

	"github.com/acme/outbound-call-campaign/internal/app"
	"github.com/acme/outbound-call-campaign/internal/api/handlers"
)

// Server wraps the Fiber application.
type Server struct {
	app    *fiber.App
	deps   *app.Container
	handlers *handlers.HandlerSet
}

// NewServer constructs a new HTTP server.
func NewServer(deps *app.Container, handlers *handlers.HandlerSet) *Server {
	cfg := fiber.Config{
		ReadTimeout:  deps.Config.HTTP.ReadTimeout,
		WriteTimeout: deps.Config.HTTP.WriteTimeout,
		IdleTimeout:  deps.Config.HTTP.IdleTimeout,
		ErrorHandler: handlers.ErrorHandler,
	}

	app := fiber.New(cfg)
	app.Use(otelfiber.Middleware())
	handlers.Register(app)

	return &Server{app: app, deps: deps, handlers: handlers}
}

// Start begins serving HTTP traffic.
func (s *Server) Start(ctx context.Context) error {
	addr := fmt.Sprintf(":%d", s.deps.Config.HTTP.Port)
	go func() {
		<-ctx.Done()
		_ = s.Shutdown()
	}()
	return s.app.Listen(addr)
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return s.app.ShutdownWithContext(ctx)
}
