# 45 — Reassignment & Failure Handling

## Objective
Recover gracefully from driver rejection, cancellation, stale location and operational failures.

## Reassignment Triggers
- offer rejected
- offer expired
- driver cancels
- driver goes offline
- driver becomes stale
- vehicle becomes invalid
- operational override

## Flow
```text
Assigned
  ↓
Failure
  ↓
Release Reservation
  ↓
Record Failure
  ↓
SEARCHING
  ↓
Dispatch Again
```

## Failure Record
Store:
```text
job_id
driver_id
reason
stage
timestamp
metadata
```

## Driver Cancellation
Customer should receive a meaningful update.

The system should determine whether:
- reassign automatically
- cancel job
- escalate to support

## Customer Communication
Avoid showing internal failure codes.

Map to customer-safe messages:
```text
"Your driver became unavailable. We're finding another driver."
```

## Repeated Failure
If a job fails repeatedly:
- widen search
- alert operations
- consider customer cancellation
- prevent endless retry

## Definition of Done
A job can survive normal driver failures without duplicate assignments or lost state.
