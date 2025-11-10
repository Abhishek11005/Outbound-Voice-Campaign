package common

import (
	"encoding/base64"
	"fmt"
)

// EncodeBase64 encodes bytes to URL-safe base64 string.
func EncodeBase64(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

// DecodeBase64 decodes URL-safe base64 string.
func DecodeBase64(s string) ([]byte, error) {
	data, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("decode base64: %w", err)
	}
	return data, nil
}
