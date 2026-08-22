# 35 — Booking & Job Lifecycle API

## Objective
Expose the customer workflow for creating, confirming, tracking and cancelling jobs.

## Quote
```http
POST /api/v1/quotes
```

The server validates service type, locations, requirements and eligible vehicle classes.

## Create Job
```http
POST /api/v1/jobs
Idempotency-Key: <unique-key>
```

Verify:
- authenticated requester
- valid quote
- quote ownership
- quote expiration
- requirements
- service availability

## Retrieval
```http
GET /api/v1/jobs
GET /api/v1/jobs/{id}
```
Results are always scoped to the authenticated user or authorized merchant/admin.

## Cancellation
```http
POST /api/v1/jobs/{id}/cancel
```

The backend determines whether cancellation is allowed, applicable fee, driver compensation and refund.

## Driver Commands
```http
POST /driver/jobs/{id}/accept
POST /driver/jobs/{id}/reject
POST /driver/jobs/{id}/arrive
POST /driver/jobs/{id}/start
POST /driver/jobs/{id}/complete
```

Every command validates assignment ownership and state.

## Customer Events
```text
job.created
job.searching
job.assigned
driver.arriving
driver.arrived
job.started
job.near_destination
job.completed
job.cancelled
```

## Completion
Ride completion finalizes applicable price/payment and creates driver earnings.

Delivery completion verifies proof/OTP or COD requirements before finalization.

## Definition of Done
Customer can quote, confirm, track and complete a job, with correct cancellation and receipt behavior.
