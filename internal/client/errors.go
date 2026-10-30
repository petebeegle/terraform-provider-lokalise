// Copyright (c) 2026 Pete Beegle
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"encoding/json"
	"errors"
	"fmt"
)

// APIError represents a non-2xx response from the Lokalise API.
type APIError struct {
	StatusCode int
	Message    string
}

// Error implements the error interface.
func (e *APIError) Error() string {
	return fmt.Sprintf("lokalise: %d %s", e.StatusCode, e.Message)
}

// errorBody is the JSON shape of a Lokalise API error response.
type errorBody struct {
	Error struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Details json.RawMessage `json:"details"`
	} `json:"error"`
}

// IsNotFound reports whether err is an *APIError with StatusCode 404.
func IsNotFound(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == 404
	}
	return false
}
