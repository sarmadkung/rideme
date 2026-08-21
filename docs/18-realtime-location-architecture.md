# Realtime & Location Architecture

## WebSocket Events

```text
job.assigned
job.accepted
job.status_changed
driver.location
driver.online
driver.offline
quote.updated
payment.updated
support.updated
```

## Location Flow

```text
Driver
 -> Native GPS
 -> client filtering
 -> WebSocket
 -> location gateway
 -> Redis current state
 -> NATS
 -> dispatch/tracking
```

Redis stores current driver state, active job, geospatial availability and short-lived locks. PostgreSQL remains durable source of truth.

Scale horizontally with multiple WebSocket gateways and geographically partitioned streams when required.

Authorize every realtime subscription and never expose arbitrary driver location.
