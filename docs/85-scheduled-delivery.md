# 85 — Scheduled Delivery

## Objective
Allow customers and businesses to request future delivery windows.

## Scheduling
Support:
```text
requested_at
scheduled_date
window_start
window_end
```

## Planning
Do not dispatch too early.

Use:
```text
scheduled_start
- preparation buffer
- pickup travel estimate
- dispatch buffer
```

## Driver Reservation
For high-value scheduled jobs, support future driver commitment while preventing excessive early locking of supply.

## Late Risk
Monitor:
- preparation delay
- driver availability
- traffic/ETA
- vehicle readiness

## Rescheduling
Allow controlled rescheduling before operational commitment.

## Definition of Done
Scheduled jobs enter dispatch at the appropriate time and can recover from supply or preparation problems.
