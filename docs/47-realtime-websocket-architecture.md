# 47 — Realtime WebSocket Architecture

## Objective
Deliver live job and driver state without turning WebSocket connections into the system of record.

## Architecture
```text
Client
  ↕ WebSocket
Realtime Gateway
  ↕
NATS / Application Events
  ↕
Domain Services
  ↕
PostgreSQL / Redis
```

## Client Channels
Conceptually:
```text
user:{id}
driver:{id}
job:{id}
merchant:{id}
admin:operations
```

Authorization is mandatory before subscription.

## Event Envelope
```json
{
  "event_id": "...",
  "type": "job.assigned",
  "version": 1,
  "occurred_at": "...",
  "resource_id": "...",
  "payload": {}
}
```

## Reconnect
Client sends last known event/checkpoint where supported.

Server should return authoritative state after reconnect.

Do not rely solely on replaying every event.

## Heartbeat
Use ping/pong and connection health.

## Backpressure
Slow clients must not block the system.

Apply:
- bounded buffers
- event coalescing for high-frequency location
- disconnect/reconnect policies

## Location
Do not broadcast every raw GPS point to every subscriber.

Throttle and transform location events according to consumer needs.

## Security
- authenticated connection
- authorization per channel
- no arbitrary channel subscription
- rate limits
- connection limits

## Definition of Done
Realtime updates work on reconnect, unauthorized subscriptions fail, and slow clients cannot destabilize the backend.
