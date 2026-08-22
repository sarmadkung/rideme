# 37 — Phase 3 Jobs, Quotes & Booking Engineering Tickets

## Goal
Build the first complete customer-to-booking workflow on top of the Phase 2 identity and vehicle foundation.

## JOB-001 — Job Domain
Implement job entity, types, stops, requirements and status history.

**Acceptance:** all initial services share the Job lifecycle.

## JOB-002 — State Machine
Implement one authoritative backend transition component.

**Acceptance:** invalid transitions are rejected and tested.

## JOB-003 — Quote Model
Implement quote, pricing snapshot, expiration and pricing version.

**Acceptance:** confirmed jobs retain original quote data.

## JOB-004 — Pricing Configuration
Implement configurable base, distance, time, minimum fare, service, loading, waiting and bounded demand adjustments.

**Acceptance:** rate changes do not require code deployment.

## JOB-005 — Route Provider
Create a replaceable route-provider interface returning distance and duration.

**Acceptance:** pricing does not depend on a specific mapping vendor.

## JOB-006 — Quote API
Implement `POST /api/v1/quotes`.

**Acceptance:** invalid locations, unsupported requirements and unavailable services are rejected correctly.

## JOB-007 — Create Job API
Implement `POST /api/v1/jobs` with idempotency.

**Acceptance:** retries cannot create duplicate jobs.

## JOB-008 — Job Retrieval
Implement list/detail APIs with strict authorization.

## JOB-009 — Customer Cancellation
Implement state-aware cancellation and configurable fees.

## JOB-010 — Driver Job Commands
Implement accept, reject, arrive, start and complete.

## JOB-011 — Assignment Timeout
Prepare job lifecycle for dispatch offers and reservations.

## JOB-012 — Realtime Job Events
Publish authorized job lifecycle events.

## JOB-013 — Customer Booking UI
```text
Home -> Service -> Pickup -> Destination -> Requirements
     -> Quote -> Confirm -> Searching -> Tracking
```

## JOB-014 — Driver Job UI
```text
Online -> Job Offer -> Accept -> Pickup -> Start -> Complete
```

## JOB-015 — Receipt
Create final receipt with service, route/time data, price breakdown, payment, driver, vehicle and timestamps.

## JOB-016 — End-to-End Test
```text
Customer login
 -> quote
 -> confirm
 -> searching
 -> test driver assignment
 -> accept
 -> start
 -> complete
 -> receipt
```

## Phase 3 Exit Criteria
A customer can create and complete a Ride/Delivery job using a verified driver and verified vehicle.

Next phase: **Dispatch Engine**.
