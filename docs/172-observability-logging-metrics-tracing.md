# 172 — Observability: Logs, Metrics & Tracing

## Objective
Make production failures diagnosable.

## Three Pillars
```text
Logs
Metrics
Traces
```

## Logs
Structured JSON-style logs should include:
```text
timestamp
service
level
request_id
correlation_id
message
```

Do not log passwords, tokens or unnecessary sensitive data.

## Metrics
Track:
- request rate
- latency
- errors
- queue depth
- database performance
- realtime connections

## Tracing
Propagate correlation/trace IDs across service boundaries.

## Definition of Done
An operational incident can be traced from user request to affected service and downstream dependency.
