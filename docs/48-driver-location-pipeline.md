# 48 — Driver Location Pipeline

## Objective
Move driver location from the mobile device into operational state safely and efficiently.

## Pipeline
```text
Native Location API
      ↓
React Native Location Layer
      ↓
Authenticated Transport
      ↓
Realtime Gateway
      ↓
Location Validator
      ↓
Redis Current State
      ↓
Dispatch / Tracking
```

## Native Strategy
React Native remains the application layer.

Use native modules when platform-specific background location, battery optimization or hardware behavior requires it.

Do not create separate screens or business logic for iOS and Android unless UI/behavior genuinely differs.

Keep:
```text
shared domain logic
shared state
shared API contract
```

Platform-specific code should be isolated behind interfaces.

## Location Payload
Include:
- latitude
- longitude
- accuracy
- heading
- speed
- timestamp

## Validation
Detect:
- impossible coordinates
- future timestamps
- stale timestamps
- impossible speed
- sudden unrealistic jumps

## Sampling
Adaptive frequency based on driver state.

Avoid maximum-frequency GPS all the time.

## Historical Tracking
Do not write every point directly into PostgreSQL synchronously.

Use a buffered/asynchronous pipeline for historical analytics where required.

## Privacy
Location collection must be:
- disclosed
- minimized
- access-controlled
- retained according to policy

## Definition of Done
Current driver state updates reliably and battery/network usage is acceptable in field testing.
