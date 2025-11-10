package handlers

import (
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/acme/outbound-call-campaign/internal/domain"
	callsvc "github.com/acme/outbound-call-campaign/internal/service/call"
)

type triggerCallRequest struct {
	CampaignID  string                 `json:"campaign_id"`
	PhoneNumber string                 `json:"phone_number"`
	Metadata    map[string]any         `json:"metadata"`
}

func (h *HandlerSet) triggerCall(ctx *fiber.Ctx) error {
	var req triggerCallRequest
	if err := ctx.BodyParser(&req); err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid request body")
	}

	input := callsvc.TriggerCallInput{
		PhoneNumber: req.PhoneNumber,
		Metadata:    req.Metadata,
	}

	if req.CampaignID != "" {
		id, err := uuid.Parse(req.CampaignID)
		if err != nil {
			return fiber.NewError(http.StatusBadRequest, "invalid campaign id")
		}
		input.CampaignID = &id
	}

	callRecord, err := h.calls.TriggerCall(ctx.Context(), input)
	if err != nil {
		return translateError(err)
	}

	return ctx.Status(http.StatusAccepted).JSON(toCallResponse(callRecord))
}

func (h *HandlerSet) getCall(ctx *fiber.Ctx) error {
	id, err := uuid.Parse(ctx.Params("id"))
	if err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid call id")
	}

	record, err := h.calls.GetCall(ctx.Context(), id)
	if err != nil {
		return translateError(err)
	}

	return ctx.Status(http.StatusOK).JSON(toCallResponse(record))
}

func toCallResponse(call *domain.Call) callResponse {
	return callResponse{
		ID:           call.ID,
		CampaignID:   call.CampaignID,
		PhoneNumber:  call.PhoneNumber,
		Status:       call.Status,
		AttemptCount: call.AttemptCount,
		ScheduledAt:  call.ScheduledAt,
		CreatedAt:    call.CreatedAt,
		UpdatedAt:    call.UpdatedAt,
		LastError:    call.LastError,
	}
}
