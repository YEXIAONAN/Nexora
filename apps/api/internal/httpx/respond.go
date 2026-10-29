package httpx

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/reqctx"
)

type errorEnvelope struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code      string            `json:"code"`
	Message   string            `json:"message"`
	Fields    map[string]string `json:"fields,omitempty"`
	RequestID string            `json:"request_id,omitempty"`
}

// Data writes a successful JSON response using the standard data envelope.
func Data(w http.ResponseWriter, status int, payload any) {
	writeJSON(w, status, map[string]any{"data": payload})
}

// JSON writes an arbitrary JSON payload.
func JSON(w http.ResponseWriter, status int, payload any) {
	writeJSON(w, status, payload)
}

// WriteError maps err to the public error envelope and writes it. Unknown
// errors become a generic 500 response; their details stay in the logs.
func WriteError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		apiErr = Internal(err)
	}

	requestID := reqctx.RequestID(r.Context())
	if apiErr.Status >= http.StatusInternalServerError {
		cause := apiErr.Unwrap()
		if cause == nil {
			cause = err
		}
		if logger != nil {
			logger.Error("request failed",
				"code", apiErr.Code,
				"status", apiErr.Status,
				"error", cause.Error(),
				"method", r.Method,
				"path", r.URL.Path,
				"request_id", requestID,
			)
		}
	}

	writeJSON(w, apiErr.Status, errorEnvelope{Error: errorDetail{
		Code:      apiErr.Code,
		Message:   apiErr.Message,
		Fields:    apiErr.Fields,
		RequestID: requestID,
	}})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
