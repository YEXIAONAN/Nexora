package httpx

import (
	"encoding/json"
	"io"
	"log/slog"
	"mime"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// DecodeBody reads a size-limited JSON object with unknown fields rejected.
func DecodeBody(w http.ResponseWriter, r *http.Request, logger *slog.Logger, dst any) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		WriteError(w, r, logger, BadRequest("the request body must use Content-Type: application/json"))
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		WriteError(w, r, logger, BadRequest("the request body must be a valid JSON object with known fields"))
		return false
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		WriteError(w, r, logger, BadRequest("the request body must contain exactly one JSON object"))
		return false
	}
	return true
}

// ParseUUIDParam validates a UUID path parameter.
func ParseUUIDParam(w http.ResponseWriter, r *http.Request, logger *slog.Logger, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		WriteError(w, r, logger, BadRequest("the "+name+" path parameter must be a valid UUID"))
		return uuid.UUID{}, false
	}
	return id, true
}
