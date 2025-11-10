package handlers

import (
	"errors"
	"net/http"

	"github.com/gofiber/fiber/v2"

	"github.com/acme/outbound-call-campaign/internal/repository"
	apperrors "github.com/acme/outbound-call-campaign/pkg/errors"
)

func translateError(err error) error {
	if err == nil {
		return nil
	}

	switch {
	case errors.Is(err, apperrors.ErrValidation):
		return fiber.NewError(http.StatusBadRequest, err.Error())
	case errors.Is(err, repository.ErrNotFound) || errors.Is(err, apperrors.ErrNotFound):
		return fiber.NewError(http.StatusNotFound, "resource not found")
	case errors.Is(err, apperrors.ErrConflict) || errors.Is(err, repository.ErrConflict):
		return fiber.NewError(http.StatusConflict, err.Error())
	case errors.Is(err, apperrors.ErrQuotaExceeded):
		return fiber.NewError(http.StatusTooManyRequests, err.Error())
	case errors.Is(err, apperrors.ErrUnavailable):
		return fiber.NewError(http.StatusServiceUnavailable, err.Error())
	default:
		return err
	}
}
