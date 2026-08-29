---
name: react-native-platform
description: Shared React Native + Expo architecture for the customer and driver apps — what belongs in shared TS, and why business logic must never be duplicated per platform. Use when building mobile screens, navigation, or app-level infrastructure.
---

# Purpose

Keep one shared application implementation across iOS and Android, with native code confined to what genuinely needs it.

# When to Use

Mobile screens, navigation, app shell, data layer, or any question of where mobile logic belongs.

# The Layering (`docs/17`)

```text
Shared React Native (screens, navigation, UI, application flows)
        ↓
Platform abstraction (TypeScript interface)
        ↓
iOS / Android native implementation — only where required
```

React Native owns screens, navigation, UI, and application flows. Swift/Kotlin own platform capabilities: background location, sensors, biometrics, secure storage, specialized navigation.

# Rules

- **Do not write separate iOS and Android screens.** A platform difference is usually a styling or capability detail, not a reason to fork a screen. Forked screens drift and double the bug surface.
- **Never duplicate pricing, dispatch, or job rules in native code** (`docs/17`). Those rules are server-side; the app displays outcomes.
- **The backend is authoritative.** The app never computes a fare, decides eligibility, or sets a job status.
- Both apps share what is genuinely shared through `packages/` — types, validation, API client, config (`docs/09`).
- Expo development builds are used where custom native modules are required (`docs/17`).
- Apps must tolerate intermittent connectivity by design (`mobile-offline-sync`), not as an afterthought.

# App Composition

**Customer app**: auth, navigation, API client, query cache, realtime, offline handling, push, deep links, location, maps.

**Driver app**: auth, navigation, API client, realtime, location, background execution, push, maps, job state, availability.

The driver app's background execution and continuous location are the main sources of genuine native requirement (`native-module-boundary`, `mobile-location`).

# Workflow

1. Build the screen or flow once, in shared TypeScript.
2. If a platform capability is needed, define the TS interface first (`native-module-boundary`).
3. Consume server state through the shared API client; do not re-derive business values.
4. Handle loading, error, offline, and stale states — these are the normal cases on mobile, not edge cases.

# Verification

Level 1–2 for a screen or component. Level 4 when it crosses into realtime or a server workflow.

Do not run mobile E2E for a local UI change (`verification-lite`).

# Blocking Conditions

- A requirement seems to need per-platform business logic → stop; that logic belongs on the server.
- A native capability is needed but the platform abstraction is undefined → define the interface before implementing either side.

# Relevant Documentation

`docs/17-native-mobile-architecture.md` · `docs/12-technical-blueprint.md` · `docs/237-driver-mobile-architecture.md` · `docs/241-customer-mobile-architecture.md` · `docs/372-react-native-coding-standards.md` · `docs/433`–`435` (app shells) · `docs/179-react-native-and-react-testing.md`
