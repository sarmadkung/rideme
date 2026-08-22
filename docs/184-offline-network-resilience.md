# 184 — Offline & Network Resilience

## Objective
Make mobile workflows robust under unstable connectivity.

## Scenarios
- no network
- slow network
- reconnect
- duplicate request
- app killed during operation
- stale cache
- websocket disconnect

## Strategy
Use:
```text
local state
retry
idempotency
server reconciliation
```

## Driver
Critical state such as job assignment should reconcile with authoritative server state after reconnect.

## Customer
Cart and draft data may be persisted locally where appropriate.

## Definition of Done
Temporary connectivity loss does not corrupt orders, jobs or user-visible state.
