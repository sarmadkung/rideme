---
name: mobile-location
description: Device-side location — GPS acquisition, filtering, batching, background execution, and permissions. Use for driver location tracking, background updates, or any work where battery cost and location accuracy trade off.
---

# Purpose

Deliver location good enough for dispatch and tracking without destroying battery life or over-collecting.

# When to Use

Driver location tracking, background execution, location permissions, tracking sessions on device.

# The Pipeline (`docs/17`, `docs/18`)

```text
GPS → native filtering → batching → React Native → WebSocket → Go → Redis/NATS
```

Filtering and batching happen **on the device**, in native code, before anything is sent. This is the whole point of the design: raw high-frequency GPS streamed to the server wastes battery, bandwidth, and storage without improving dispatch.

# Rules

- **Filter and batch natively.** Do not send every fix. Drop fixes that are inaccurate, redundant, or below a movement threshold.
- **Every fix carries timestamp, accuracy, speed, heading** (`docs/16`) — the server needs these to judge freshness and reject stale data.
- **Background execution is a driver-app requirement**, and it is platform-specific: iOS and Android differ substantially in what they permit and when they suspend. Handle suspension and resumption explicitly (`docs/239`).
- **Permissions are a user flow, not a call.** Denied, "while in use" only, and revoked-later are all normal states the app must handle without breaking.
- **Collect the minimum needed** (`location-tracking`). Tracking frequency should reflect job state — a driver on a trip is not the same as a driver idle and available.
- Never log or cache location beyond what the feature needs; on-device caches are a privacy surface too (`docs/317`).
- No business logic in the native layer (`native-module-boundary`).

# Workflow

1. Determine the accuracy and frequency the consumer actually needs — dispatch, live tracking, and history differ.
2. Configure native tracking to that need, not to the maximum available.
3. Filter and batch before crossing into JavaScript.
4. Send over the WebSocket; tolerate disconnect by buffering briefly and dropping stale fixes rather than replaying an old trail.
5. Handle permission changes and background suspension as normal paths.

# Verification

Level 5 — location affects dispatch, privacy, and battery.

Required: permission denied and later granted, background suspension and resume, connectivity loss and reconnect, stale fixes discarded rather than replayed, batching honoured, and a battery measurement over a realistic session (`mobile-performance`).

# Blocking Conditions

- Required tracking frequency per job state is undocumented → `BLOCKED_TASKS.md`; it is a battery-versus-accuracy product decision.
- On-device retention of location history unspecified → privacy decision; ask.

# Relevant Documentation

`docs/17-native-mobile-architecture.md` · `docs/18-realtime-location-architecture.md` · `docs/99-react-native-native-location-strategy.md` · `docs/102-location-privacy-and-security.md` · `docs/239-driver-mobile-location-and-background.md` · `docs/182-mobile-performance-and-battery.md` · `docs/335-location-testing.md`
