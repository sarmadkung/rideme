---
name: realtime-architecture
description: WebSocket gateway rules — event contracts, subscription authorization, and client resynchronization after disconnect or missed events. Use when adding a realtime event, changing a channel, or when a client needs live job or driver state.
---

# Purpose

Deliver live state without making the socket a second source of truth.

# When to Use

- Adding or changing a WebSocket event or channel.
- Client needs live tracking, job status, or driver presence.
- Debugging state that diverges between client and server.

# Rules

- **Realtime events are not the source of truth.** PostgreSQL is. Events are hints that state changed; the authoritative value is fetched or reconciled (`docs/18`).
- **Every client must recover from**: disconnect, duplicate event, missing event, out-of-order event, stale state. Design the recovery path before the happy path — mobile networks guarantee you will need it.
- **Snapshot + delta.** On connect or resubscribe the client fetches a full snapshot, then applies deltas. A client that can only apply deltas will drift and cannot be repaired without a restart.
- **Authorize every subscription** (`docs/18`). A customer may follow their own job's driver; nobody subscribes to arbitrary driver location. This is a privacy boundary, not a filter.
- **Never expose raw driver location** outside an active job relationship (`location-tracking`).
- Scale with multiple gateways and partitioned streams; do not hold business state in gateway memory.

# Event Surface (`docs/18`)

```text
job.assigned       job.accepted      job.status_changed
driver.location    driver.online     driver.offline
quote.updated      payment.updated   support.updated
```

Add events deliberately and version their payloads (`docs/376`).

# Workflow

1. Confirm the state genuinely needs to be live — polling is cheaper and simpler for slow-changing data.
2. Choose an existing event before adding one.
3. Define the channel and its authorization rule.
4. Implement snapshot fetch and delta application on the client.
5. Test disconnect → reconnect → resync produces state identical to a fresh load.

# Verification

Level 4 — realtime crosses client and server. Level 5 if the channel carries location or payment state.

Required cases: reconnect resync, duplicate event, dropped event, out-of-order arrival, unauthorized subscription attempt, and one comparison of post-resync state against a cold load.

# Blocking Conditions

- An event would carry data the subscriber is not authorized to see → stop. Reshape the payload.
- Resynchronization has no defined snapshot endpoint → build it first; delta-only is not shippable.

# Relevant Documentation

`docs/18-realtime-location-architecture.md` · `docs/200-realtime-event-contracts.md` · `docs/387-realtime-gateway.md` · `docs/388-realtime-presence.md` · `docs/389-realtime-resynchronization.md` · `docs/425-websocket-channel-spec.md` · `docs/426-websocket-event-spec.md` · `docs/428-realtime-client-state.md`
