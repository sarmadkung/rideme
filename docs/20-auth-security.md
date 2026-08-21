# Authentication, Authorization & Security

## Authentication

Initial: phone OTP. Later: Google and Apple through an auth abstraction.

## Roles

`CUSTOMER`, `DRIVER`, `MERCHANT`, `SUPPORT`, `ADMIN`, `SUPER_ADMIN`.

Use role-based and resource-level authorization.

## Security Controls

- Short-lived access tokens
- Refresh token rotation
- Secure platform token storage
- Rate limiting
- Input validation
- Idempotency
- Audit logging
- Structured errors

Sensitive operations such as payout changes, phone changes, withdrawals and privilege changes require stronger verification.

Location data must have purpose-based access and controlled retention.

Fraud signals include GPS spoofing, duplicate accounts, collusion, fake completion, abnormal cancellations and payment anomalies.
