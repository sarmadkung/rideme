---
name: location-tracking
description: Server-side location ingestion, storage, retention, and visibility rules. Use when handling driver location updates, tracking sessions, geospatial queries, or deciding who may see whose position — location is sensitive data with access rules, not just coordinates.
---

# Purpose

Move location from device to dispatch efficiently, and expose it only to those entitled to see it.

# When to Use

- Ingesting or storing driver location.
- Tracking sessions and customer-facing driver position.
- Geospatial queries, zones, geofences.
- Retention or privacy questions about location history.

# Rules

- **Pipeline** (`docs/18`): `Driver → native GPS → client filtering → WebSocket → location gateway → Redis current state → NATS → dispatch/tracking`. Redis holds current position; PostgreSQL holds durable history.
- **Filter on the device** (`native-module-boundary`, `mobile-location`). Do not stream raw high-frequency GPS to the server — it costs battery, bandwidth, and storage for no gain.
- **Records carry timestamp, accuracy, speed, heading** (`docs/16`). Consumers must be able to judge freshness.
- **Stale locations are excluded from dispatch** (`docs/16`). Define staleness explicitly; do not leave it implicit.
- **Visibility is a relationship, never a role.** A customer sees their assigned driver's location during an active job — not before assignment, not after completion. Nobody subscribes to arbitrary driver position (`docs/18`).
- **Collect the minimum, retain deliberately.** High-frequency history is not kept indefinitely without a stated reason (`docs/102`, `docs/497`). Retention is a policy decision — if undocumented, it is a blocking question, not a default.
- Location data is never exposed in logs, analytics events, or error reports at full precision.

# Workflow

1. Confirm which consumer needs the data and at what precision and frequency — these differ for dispatch, live tracking, and history.
2. Ingest through the gateway; write current state to Redis, durable history to Postgres/PostGIS.
3. Apply the visibility rule at the subscription boundary, not in the client.
4. Index geometry columns for the proximity queries actually used.
5. Apply the retention policy at write time where possible.

# Verification

Level 5 — location is sensitive and feeds dispatch.

Required: unauthorized subscription rejected, visibility ends when the job ends, stale location excluded from dispatch candidates, ingestion survives duplicate and out-of-order updates, retention actually deletes.

# Blocking Conditions

- Retention period for a location table is undocumented → `BLOCKED_TASKS.md`. Storing indefinitely by default is a privacy decision you should not make silently.
- A feature requires exposing location outside an active job relationship → stop; product and privacy decision.

# Relevant Documentation

`docs/18-realtime-location-architecture.md` · `docs/48-driver-location-pipeline.md` · `docs/98-driver-location-tracking.md` · `docs/102-location-privacy-and-security.md` · `docs/103-location-events-and-realtime.md` · `docs/229-location-architecture.md` · `docs/231-tracking-sessions.md` · `docs/335-location-testing.md` · `docs/497-location-history-policy.md`
