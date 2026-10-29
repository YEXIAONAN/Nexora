// Package httpx contains transport-level helpers shared by HTTP handlers:
// the error envelope, JSON responses and request-scoped error mapping.
package httpx

import (
	"errors"
	"fmt"
	"net/http"
)

// APIError is an error that carries an HTTP status and a machine-readable code.
type APIError struct {
	Status  int
	Code    string
	Message string
	Fields  map[string]string
	cause   error
}

func (e *APIError) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %v", e.Code, e.cause)
	}
	return e.Code + ": " + e.Message
}

// Unwrap exposes the wrapped cause to errors.Is / errors.As.
func (e *APIError) Unwrap() error { return e.cause }

// WithCause attaches an internal cause. The cause is logged but never sent to
// the client.
func (e *APIError) WithCause(cause error) *APIError {
	e.cause = cause
	return e
}

// BadRequest reports a malformed or semantically invalid request.
func BadRequest(message string) *APIError {
	return &APIError{Status: http.StatusBadRequest, Code: "bad_request", Message: message}
}

// ValidationFailed reports field-level validation failures.
func ValidationFailed(fields map[string]string) *APIError {
	return &APIError{
		Status:  http.StatusUnprocessableEntity,
		Code:    "validation_failed",
		Message: "the request contains invalid fields",
		Fields:  fields,
	}
}

// Unauthorized reports a missing or invalid authenticated identity.
func Unauthorized(message string) *APIError {
	return &APIError{Status: http.StatusUnauthorized, Code: "unauthorized", Message: message}
}

// Forbidden reports that the identity may not perform the operation.
func Forbidden(message string) *APIError {
	return &APIError{Status: http.StatusForbidden, Code: "forbidden", Message: message}
}

// NotFound reports a missing resource. Callers must not use it to hide other
// failures.
func NotFound(message string) *APIError {
	return &APIError{Status: http.StatusNotFound, Code: "not_found", Message: message}
}

// Conflict reports a state conflict such as a duplicate unique value.
func Conflict(message string) *APIError {
	return &APIError{Status: http.StatusConflict, Code: "conflict", Message: message}
}

// Unavailable reports a dependency that is currently unavailable.
func Unavailable(message string) *APIError {
	return &APIError{Status: http.StatusServiceUnavailable, Code: "service_unavailable", Message: message}
}

// Internal wraps an unexpected error. The original error is preserved for logs.
func Internal(cause error) *APIError {
	return &APIError{
		Status:  http.StatusInternalServerError,
		Code:    "internal_error",
		Message: "an unexpected error occurred",
		cause:   cause,
	}
}

// IsAPIError reports whether err is (or wraps) an APIError.
func IsAPIError(err error) bool {
	var target *APIError
	return errors.As(err, &target)
}
