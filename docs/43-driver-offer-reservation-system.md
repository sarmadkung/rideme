# 43 — Driver Offer & Reservation System

## Objective
Prevent double assignment while giving drivers a short window to accept work.

## States
```text
CANDIDATE
RESERVED
OFFERED
ACCEPTED
REJECTED
EXPIRED
RELEASED
```

## Reservation
Before sending an offer:
```text
job -> reserve driver
```

Reservation has an expiry timestamp.

The driver must still pass an authoritative availability check.

## Offer
Offer contains:
- job type
- pickup area
- estimated distance/time
- expected earnings where policy allows
- expiration
- relevant requirements

Avoid exposing sensitive customer information before acceptance.

## Acceptance
Acceptance must be atomic.

```text
BEGIN
  verify offer
  verify reservation
  verify driver eligibility
  assign job
  consume reservation
COMMIT
```

If any step fails, the driver does not win the assignment.

## Timeout
When an offer expires:
```text
offer -> EXPIRED
reservation -> RELEASED
job -> SEARCHING
```

## Batch Offers
MVP should prefer one driver at a time for high-confidence jobs.

Controlled small-batch offers can be introduced later where latency requires it.

## Definition of Done
- double acceptance is impossible
- expired offers cannot be accepted
- reservations release reliably
- every offer is auditable
