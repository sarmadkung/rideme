package httpx

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/sarmadkung/rideme/services/api/pkg/observability"
)

// ErrorBody is the failure envelope. Its shape is mirrored by
// @platform/types.ApiErrorBody and validated by @platform/validation.
type ErrorBody struct {
	Code      Code              `json:"code"`
	Message   string            `json:"message"`
	RequestID string            `json:"request_id"`
	Details   map[string]string `json:"details,omitempty"`
}

// WriteJSON renders a success payload.
func WriteJSON(w http.ResponseWriter, r *http.Request, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		// The status line is already sent; all that is left is to record it.
		observability.LoggerFrom(r.Context()).Error("failed to encode response",
			slog.String("error", err.Error()))
	}
}

// WriteError renders a failure. The client receives the code and message; the
// cause is logged and never serialised.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	appErr := AsError(err)
	status := StatusFor(appErr.Code)
	requestID := observability.RequestIDFrom(r.Context())

	logger := observability.LoggerFrom(r.Context())
	attrs := []any{
		slog.String("code", string(appErr.Code)),
		slog.Int("status", status),
		slog.String("error", appErr.Error()),
	}
	if status >= 500 {
		logger.Error("request failed", attrs...)
	} else {
		logger.Warn("request rejected", attrs...)
	}

	WriteJSON(w, r, status, ErrorBody{
		Code:      appErr.Code,
		Message:   appErr.Message,
		RequestID: requestID,
		Details:   appErr.Details,
	})
}

// PanicHandler adapts the error envelope for observability.Recover, so a panic
// produces the same response shape as any other failure.
func PanicHandler(w http.ResponseWriter, r *http.Request, _ any) {
	WriteError(w, r, ErrInternal)
}

// NotFoundHandler answers unrouted paths in the platform's envelope rather than
// net/http's plain-text default.
func NotFoundHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, r, NotFound("no route matches "+r.Method+" "+r.URL.Path))
	})
}
