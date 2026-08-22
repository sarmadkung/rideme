# 123 — SMS, Email & OTP

## Objective
Provide provider-independent messaging for transactional communication and verification.

## SMS Uses
- OTP
- critical delivery updates
- safety workflows
- selected support communication

## Email Uses
- receipts
- account notices
- settlement reports
- support updates
- verification where appropriate

## OTP
Properties:
```text
short-lived
single-use
rate-limited
purpose-bound
```

Never store OTP values in plaintext longer than operationally required.

## Provider Adapter
```text
Messaging Service
 ├── SMS Provider
 └── Email Provider
```

## Fallback
Critical workflows can support configured provider fallback.

## Definition of Done
SMS/email providers can be replaced without changing domain logic.
