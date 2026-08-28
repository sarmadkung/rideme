package httpx

// This file holds the request/response conventions every endpoint shares.
// They are contract, not transport plumbing: a client can rely on them without
// reading a handler.

// APIVersionPrefix is the versioned base path from document 14. Every domain
// route lives under it; only the health endpoints sit outside, because an
// operator probing liveness should not have to know the API version.
const APIVersionPrefix = "/api/v1"

// IdempotencyKeyHeader carries the client's idempotency key.
//
// Document 14 requires it for job and payment creation; document 185 defines
// the key as "client/request id + operation scope" and lists the operations
// that must be idempotent. Naming the header here means every mutation that
// needs one spells it the same way.
//
// Enforcement belongs to the phases that introduce mutations — there are none
// yet. This is the contract, not the middleware.
const IdempotencyKeyHeader = "Idempotency-Key"

// DefaultPageLimit and MaxPageLimit bound list responses.
const (
	DefaultPageLimit = 25
	MaxPageLimit     = 100
)

// PageInfo is the pagination envelope carried by every list response.
//
// Cursor-based, not offset-based: the platform's lists are time-ordered and
// actively written to — jobs, locations, ledger entries — and an offset into a
// growing list skips and repeats rows as it shifts. A cursor does not. No
// authoritative document specifies a pagination strategy, so this is an
// engineering decision, recorded as ADR-009.
//
// NextCursor is empty on the last page. It is opaque: clients pass it back
// unmodified and never construct or parse one.
type PageInfo struct {
	NextCursor string `json:"next_cursor,omitempty"`
	Limit      int    `json:"limit"`
}

// ClampLimit brings a client-supplied page size into range. A limit of zero
// means "unspecified" and takes the default; anything above the maximum is
// capped rather than rejected, because a client asking for too much wants as
// much as it can have.
func ClampLimit(requested int) int {
	switch {
	case requested <= 0:
		return DefaultPageLimit
	case requested > MaxPageLimit:
		return MaxPageLimit
	default:
		return requested
	}
}
