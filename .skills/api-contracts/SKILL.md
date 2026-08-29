---
name: api-contracts
description: Rules every RideMe REST endpoint must satisfy — auth, validation, errors, idempotency, pagination, rate limiting, observability. Use before adding or changing any endpoint, and when tempted to create a new route rather than extend an existing one.
---

# Purpose

Keep the API surface consistent and safe, and stop endpoint sprawl.

# When to Use

- Adding, changing, or deprecating an endpoint.
- Wiring a client to the backend.
- Reviewing an API diff.

# Rules

Every endpoint defines, explicitly: **authentication · authorization · request schema · response schema · validation · error cases · pagination (for collections) · idempotency (for unsafe methods) · rate limiting (where abusable) · observability**. An endpoint missing one of these is incomplete, not "done".

- **Search before creating.** Extending an existing resource beats a new route.
- **Validate every external input server-side.** Client validation is UX, never enforcement.
- **Authorize per operation**, not per route prefix. Ownership checks (this customer's job, this driver's assignment) are authorization, and they belong on the server.
- **Never trust client-supplied** prices, permissions, statuses, driver ids, or totals. Recompute or look up.
- **Idempotency keys on unsafe mutations** (`docs/377`) — booking, payment, assignment acceptance. A retried request must not create a second Job or a second charge.
- **Uniform errors** (`docs/374`) — one shape across the API, with a stable machine-readable code. Never leak internal detail or stack traces.
- **Correlation IDs** on every request, propagated into logs and events.

# Workflow

1. Find the owning module; check the existing routes there.
2. Define request/response schemas before the handler.
3. Write the authorization rule as a sentence, then implement it.
4. Add the idempotency key if the method is unsafe.
5. Implement, with structured logging and the correlation ID.
6. Write contract tests for: success, invalid input, unauthenticated, unauthorized, duplicate/retry.

# Verification

Level 3 for a new or changed endpoint — unit plus integration plus contract tests. Level 5 for anything on payment, dispatch assignment, or auth: add duplicate-request and concurrent-request cases.

Required negative tests, always: invalid payload, missing auth, wrong-owner auth, replayed idempotency key.

# Blocking Conditions

- The authorization rule for an endpoint is not documented and not derivable → stop; ask. Guessing here is a security decision.
- A breaking contract change with live clients and no migration path → ADR plus versioning (`docs/375`).

# Relevant Documentation

`docs/14-api-specification.md` · `docs/374-api-error-standard.md` · `docs/375-api-versioning.md` · `docs/377-idempotency-standard.md` · `docs/393-rate-limiting.md` · `docs/421-endpoint-catalog.md` · `docs/422-request-response-conventions.md` · `docs/423-api-auth-matrix.md`
