package handlers

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	"github.com/acme/outbound-call-campaign/internal/app"
	campaignsvc "github.com/acme/outbound-call-campaign/internal/service/campaign"
	callsvc "github.com/acme/outbound-call-campaign/internal/service/call"
)

// HandlerSet bundles all HTTP handlers.
type HandlerSet struct {
	container *app.Container
	campaigns *campaignsvc.Service
	calls     *callsvc.Service
}

// NewHandlerSet creates a new handler bundle.
func NewHandlerSet(container *app.Container) *HandlerSet {
	services := container.Services()
	return &HandlerSet{
		container: container,
		campaigns: services.Campaign,
		calls:     services.Call,
	}
}

// Register wires all routes onto the fiber app.
func (h *HandlerSet) Register(app *fiber.App) {
	app.Get("/healthz", h.health)

	api := app.Group("/api")
	v1 := api.Group("/v1")

	campaigns := v1.Group("/campaigns")
	campaigns.Post("/", h.createCampaign)
	campaigns.Get("/", h.listCampaigns)
	campaigns.Get("/:id", h.getCampaign)
	campaigns.Put("/:id", h.updateCampaign)
	campaigns.Post("/:id/start", h.startCampaign)
	campaigns.Post("/:id/pause", h.pauseCampaign)
	campaigns.Post("/:id/complete", h.completeCampaign)
	campaigns.Get("/:id/stats", h.campaignStats)
	campaigns.Post("/:id/targets", h.addTargets)
	campaigns.Get("/:id/calls", h.listCampaignCalls)

	calls := v1.Group("/calls")
	calls.Post("/", h.triggerCall)
	calls.Get("/:id", h.getCall)
}

// ErrorHandler provides centralized error responses.
func (h *HandlerSet) ErrorHandler(ctx *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	message := err.Error()

	if fiberErr, ok := err.(*fiber.Error); ok {
		code = fiberErr.Code
		message = fiberErr.Message
	}

	if code == fiber.StatusInternalServerError {
		h.container.Logger.Error("request failed", zap.Error(err))
	}

	return ctx.Status(code).JSON(fiber.Map{
		"error":    message,
		"trace_id": ctx.GetRespHeader("Trace-Id"),
	})
}

func (h *HandlerSet) health(ctx *fiber.Ctx) error {
	healthCtx, cancel := context.WithTimeout(ctx.Context(), 2*time.Second)
	defer cancel()

	errs := make(map[string]string)

	if err := h.container.Postgres.DB().PingContext(healthCtx); err != nil {
		errs["postgres"] = err.Error()
	}

	if err := h.container.Redis.Inner().Ping(healthCtx).Err(); err != nil {
		errs["redis"] = err.Error()
	}

	if err := h.container.Scylla.Session().Query("SELECT now() FROM system.local").WithContext(healthCtx).Exec(); err != nil {
		errs["scylla"] = err.Error()
	}

	status := fiber.StatusOK
	if len(errs) > 0 {
		status = fiber.StatusServiceUnavailable
	}

	return ctx.Status(status).JSON(fiber.Map{"status": "ok", "errors": errs})
}
