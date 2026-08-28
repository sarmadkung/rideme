// Package httpx holds the platform's HTTP transport concerns: the typed error
// taxonomy from document 25 and its single, consistent mapping to status codes.
//
// Handlers return an *Error; only this package decides what that means over
// HTTP. Nothing else in the codebase writes a status code for a failure.
package httpx

import (
	"errors"
	"fmt"
	"net/http"
)

// Code is the machine-readable error code carried in every failure envelope.
// The list is mirrored in @platform/types; keeping the two in sync by hand is
// tracked as B-2 in docs/BLOCKED_TASKS.md.
type Code string

const (
	CodeNotFound     Code = "not_found"
	CodeUnauthorized Code = "unauthorized"
	CodeForbidden    Code = "forbidden"
	CodeConflict     Code = "conflict"
	CodeValidation   Code = "validation"
	CodeRateLimited  Code = "rate_limited"
	CodeUnavailable  Code = "unavailable"
	CodeInternal     Code = "internal"
)

// Error is an application error carrying a transport-independent code.
type Error struct {
	Code    Code
	Message string
	// Details carries field-level context for validation failures. It is
	// serialised to the client, so it must never hold internal state.
	Details map[string]string
	cause   error
}

func (e *Error) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.cause }

// WithCause attaches the underlying error. The cause is logged, never returned
// to the client — a database error message is an information leak.
func (e *Error) WithCause(cause error) *Error {
	clone := *e
	clone.cause = cause
	return &clone
}

// Sentinels for errors.Is comparison.
var (
	ErrNotFound     = &Error{Code: CodeNotFound, Message: "resource not found"}
	ErrUnauthorized = &Error{Code: CodeUnauthorized, Message: "authentication required"}
	ErrForbidden    = &Error{Code: CodeForbidden, Message: "not permitted"}
	ErrConflict     = &Error{Code: CodeConflict, Message: "conflicting state"}
	ErrValidation   = &Error{Code: CodeValidation, Message: "invalid request"}
	ErrRateLimited  = &Error{Code: CodeRateLimited, Message: "too many requests"}
	ErrUnavailable  = &Error{Code: CodeUnavailable, Message: "service unavailable"}
	ErrInternal     = &Error{Code: CodeInternal, Message: "internal error"}
)

// Is compares by code so errors.Is(err, ErrNotFound) matches any not-found
// error regardless of its message or cause.
func (e *Error) Is(target error) bool {
	var other *Error
	if !errors.As(target, &other) {
		return false
	}
	return e.Code == other.Code
}

func NotFound(message string) *Error     { return &Error{Code: CodeNotFound, Message: message} }
func Unauthorized(message string) *Error { return &Error{Code: CodeUnauthorized, Message: message} }
func Forbidden(message string) *Error    { return &Error{Code: CodeForbidden, Message: message} }
func Conflict(message string) *Error     { return &Error{Code: CodeConflict, Message: message} }
func Unavailable(message string) *Error  { return &Error{Code: CodeUnavailable, Message: message} }
func RateLimited(message string) *Error  { return &Error{Code: CodeRateLimited, Message: message} }
func Internal(message string) *Error     { return &Error{Code: CodeInternal, Message: message} }

func Validation(message string, details map[string]string) *Error {
	return &Error{Code: CodeValidation, Message: message, Details: details}
}

// StatusFor maps a code to its HTTP status. This is the only mapping in the
// codebase.
//
// CodeValidation maps to 422 rather than 400: 400 means the request could not
// be understood, 422 means it was understood and rejected on its content. The
// documentation does not specify a code; this is an engineering choice.
func StatusFor(code Code) int {
	switch code {
	case CodeNotFound:
		return http.StatusNotFound
	case CodeUnauthorized:
		return http.StatusUnauthorized
	case CodeForbidden:
		return http.StatusForbidden
	case CodeConflict:
		return http.StatusConflict
	case CodeValidation:
		return http.StatusUnprocessableEntity
	case CodeRateLimited:
		// Document 20 requires rate limiting on authentication. 429 is the
		// status a client can act on — back off and retry — where 409 would
		// tell it to change the request.
		return http.StatusTooManyRequests
	case CodeUnavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// AsError extracts an *Error from any error, classifying anything unrecognised
// as internal. An unclassified error must never leak its message to a client.
func AsError(err error) *Error {
	var target *Error
	if errors.As(err, &target) {
		return target
	}
	return ErrInternal.WithCause(err)
}
