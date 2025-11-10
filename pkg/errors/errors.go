package errors

import "errors"

// Sentinels for domain errors.
var (
	ErrNotFound      = errors.New("not found")
	ErrConflict      = errors.New("conflict")
	ErrValidation    = errors.New("validation error")
	ErrUnavailable   = errors.New("service unavailable")
	ErrQuotaExceeded = errors.New("quota exceeded")
)

// Is reports whether err is one of the sentinels.
func Is(err, target error) bool {
	return errors.Is(err, target)
}

// Wrap adds context to an error.
func Wrap(err error, message string) error {
	if err == nil {
		return nil
	}
	return errors.Join(errors.New(message), err)
}
