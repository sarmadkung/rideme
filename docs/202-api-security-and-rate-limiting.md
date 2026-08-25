# 202 — API Security & Rate Limiting

## Objective
Protect APIs against abuse and accidental overload.

## Controls
- authentication
- authorization
- request validation
- rate limiting
- payload limits
- timeout limits
- abuse detection
- audit logging

## Rate-limit categories
Different limits may apply to:
- login
- OTP
- registration
- quote generation
- booking
- location updates
- messages
- admin operations

## Rules
Never rely only on client-side throttling.

## Agent tasks
- Implement centralized rate-limit abstraction.
- Add endpoint-specific policies.
- Add abuse telemetry.
- Test bypass attempts.

## Acceptance criteria
Sensitive endpoints are protected and rate-limit behavior is observable.
