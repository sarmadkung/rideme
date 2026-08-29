---
name: mobile-offline-sync
description: Offline behavior for the mobile apps — local state, mutation queues, optimistic updates, reconciliation, and recovery from missed realtime events. Use when a mobile feature must survive poor connectivity, which on this platform is most of them.
---

# Purpose

Keep the apps usable and correct on unreliable networks, without letting the device become a competing source of truth.

# When to Use

Any mobile feature that mutates server state, displays live state, or must work while connectivity comes and goes.

# Rules

- **The server is authoritative; the device holds a cache.** Optimistic updates are a display convenience that the server can always overrule. When they diverge, the server wins and the UI corrects itself visibly.
- **Queued mutations must be idempotent** (`api-contracts`, `docs/377`). A queued booking replayed after reconnect must not create a second job — carry the idempotency key generated at request time, not at send time.
- **Never queue a financial mutation optimistically as if it succeeded.** Payment state is confirmed by the server, never assumed by the client (`payment-flow`).
- **Recover from missed realtime events by snapshot, not by replay** (`realtime-architecture`). On reconnect: fetch a snapshot, reconcile, then resume deltas. A client that only applies deltas will drift permanently.
- **Show real state.** Pending, offline, failed, and stale must be visible to the user. A UI that looks identical online and offline will get a driver stranded believing a job was accepted.
- Conflicts are resolved server-side. The device does not merge business state.
- Bound the queue. Unbounded offline queues replay stale intent — an hour-old job acceptance is usually wrong to send.

# Workflow

1. Classify each mutation: safe to queue, must be online, or optimistic-with-rollback.
2. Generate the idempotency key when the user acts, and persist it with the queued mutation.
3. Persist the queue durably — app kills are normal.
4. On reconnect: flush the queue, then snapshot and reconcile.
5. Surface pending and failed states in the UI.
6. Expire queued mutations that are no longer meaningful.

# Verification

Level 4; Level 5 when the queue can carry a payment or a job acceptance.

Required: mutation queued offline then flushed once (not twice), app killed with a pending queue, reconnect resync matches a cold load, stale queued mutation expires, conflicting server state overrides the optimistic update, and a payment never optimistically shown as complete.

# Blocking Conditions

- Which mutations may be queued at all is undocumented for a flow → `BLOCKED_TASKS.md`; queuing a job acceptance has real operational consequences.
- Queue expiry windows unspecified → ask rather than choosing.

# Relevant Documentation

`docs/184-offline-network-resilience.md` · `docs/344-mobile-offline-architecture.md` · `docs/345-mobile-sync-engine.md` · `docs/389-realtime-resynchronization.md` · `docs/428-realtime-client-state.md` · `docs/185-data-consistency-and-idempotency.md`
