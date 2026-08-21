# API Specification

Base path: `/api/v1`

## Authentication
```text
POST /auth/otp/request
POST /auth/otp/verify
GET  /me
```

## Customer
```text
POST /quotes
POST /jobs
GET  /jobs
GET  /jobs/{id}
POST /jobs/{id}/cancel
GET  /jobs/{id}/tracking
POST /jobs/{id}/rating
```

## Driver
```text
POST  /driver/onboarding
GET   /driver/profile
POST  /driver/vehicles
PATCH /driver/availability
GET   /driver/jobs
POST  /driver/jobs/{id}/accept
POST  /driver/jobs/{id}/reject
POST  /driver/jobs/{id}/status
GET   /driver/earnings
```

## Merchant
```text
POST /merchants
GET  /merchant/orders
POST /merchant/orders
POST /merchant/orders/bulk
GET  /merchant/deliveries
GET  /merchant/reports
```

## Admin
```text
GET /admin/jobs
GET /admin/drivers
GET /admin/vehicles
POST /admin/drivers/{id}/verify
POST /admin/vehicles/{id}/verify
PATCH /admin/pricing
GET /admin/support/cases
```

## Rules
Use Bearer authentication, resource authorization, request validation, rate limiting and `Idempotency-Key` for job/payment creation.
