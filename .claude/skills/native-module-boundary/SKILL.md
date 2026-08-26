---
name: native-module-boundary
description: Decides when a native iOS/Android module is genuinely required and how to keep it behind a TypeScript interface. Use before writing any Swift or Kotlin, and whenever someone proposes native code for something React Native can already do.
---

# Purpose

Confine native code to capabilities that truly need it, so the platform does not accumulate two divergent implementations of the same feature.

# When to Use

Before writing Swift or Kotlin. When evaluating whether a requirement needs a native module.

# The Test

A native module is justified only when the platform capability or measured performance requires it (`docs/17`). Documented legitimate cases:

- background location
- advanced location services
- high-performance native processing
- push notification integration
- secure storage
- device APIs (sensors, biometrics)

Everything else — screens, navigation, business flows, formatting, state — stays in shared TypeScript.

"It would be faster in native" is not a justification without a measurement (`mobile-performance`).

# Rules

- **The TypeScript interface is designed first**, and it is what the app depends on (`docs/17`):
  ```ts
  interface LocationService {
    startTracking(options: TrackingOptions): Promise<void>;
    stopTracking(): Promise<void>;
    getCurrentLocation(): Promise<Location>;
  }
  ```
  ```text
  TypeScript interface
         |
     Native module
      /       \
   Swift     Kotlin
  ```
- **Never put business rules in native code** (`docs/17`). No pricing, no dispatch, no job state. Native code moves data and exposes capabilities.
- **Both platforms implement the same interface** with the same semantics. A method that behaves differently on iOS and Android is a defect, not a platform quirk — encode the difference in the interface if it is real.
- Keep the native surface small. Every method is maintained twice, forever.
- Custom native modules require Expo development builds (`docs/17`).

# Workflow

1. State the capability and why RN or an existing Expo module cannot provide it.
2. Check for an existing Expo/community module before writing anything.
3. Define the TypeScript interface and its error cases.
4. Implement Swift and Kotlin to identical semantics.
5. Test through the interface, so tests are platform-agnostic.

# Verification

Level 3 minimum. Level 5 for background location or secure storage — these affect privacy, battery, and driver earnings.

Required: both platforms satisfy the same interface tests, permission denied, capability unavailable, app backgrounded and resumed, and — for location — that no business logic crept into native.

# Blocking Conditions

- The requirement can be met without native code → do not write native code.
- Semantics genuinely cannot match across platforms → surface it in the interface and record an ADR; do not hide it.
- Platform capability requires an entitlement or permission not yet approved → block.

# Relevant Documentation

`docs/17-native-mobile-architecture.md` · `docs/99-react-native-native-location-strategy.md` · `docs/239-driver-mobile-location-and-background.md` · `docs/436-driver-native-module-boundaries.md` · `docs/317-mobile-security.md`
