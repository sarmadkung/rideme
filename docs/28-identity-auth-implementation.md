# 28 — Identity & Authentication Implementation

## Objective
Implement a shared identity system for customers, drivers, merchants, support agents, and administrators.

## Initial Authentication
- Phone number + OTP
- Optional email
- Google/Apple can be added later behind the same auth abstraction.

## Roles
```text
CUSTOMER
DRIVER
MERCHANT
SUPPORT
ADMIN
SUPER_ADMIN
```

A user may hold more than one role.

## Authentication Flow
```text
Phone Number
  -> Request OTP
  -> Verify OTP
  -> Resolve/Create User
  -> Issue Access Token
  -> Issue Refresh Token
  -> Load User + Roles
```

## Core Endpoints
```text
POST /api/v1/auth/otp/request
POST /api/v1/auth/otp/verify
POST /api/v1/auth/refresh
POST /api/v1/auth/logout
GET  /api/v1/me
```

## OTP Rules
- Normalize phone numbers before lookup.
- OTPs expire quickly.
- Never store plaintext OTPs.
- Rate-limit by phone, IP, and device signals.
- Limit verification attempts.
- Prevent account enumeration.
- OTP provider must be behind an interface.

## Tokens
### Access Token
Short-lived.

Contains only minimal claims:
```text
sub
session_id
roles/version if required
iat
exp
```

### Refresh Token
- Longer-lived
- Rotated on refresh
- Stored securely
- Revocable
- Associated with a session/device

## Mobile Storage
Refresh credentials must use secure platform storage:
- iOS Keychain
- Android Keystore-backed secure storage

Do not store sensitive tokens in plain AsyncStorage.

## Authorization
Use both:
1. role authorization
2. resource authorization

Examples:
- driver can modify only their own driver profile
- merchant can access only their own merchant resources
- support permissions are narrower than admin permissions

## Sessions
Track:
```text
session_id
user_id
device_id
refresh_token_hash
created_at
last_used_at
expires_at
revoked_at
```

Allow session revocation.

## Security Events
Audit:
- login
- logout
- OTP failures
- refresh-token reuse
- phone change
- payout change
- role changes
- suspicious device changes

## Definition of Done
- OTP request/verification works
- refresh rotation works
- logout/revocation works
- mobile secure storage is integrated
- role middleware works
- resource authorization tests exist
- rate limiting exists
- auth events are audited
