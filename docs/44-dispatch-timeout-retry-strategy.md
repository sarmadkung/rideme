# 44 — Dispatch Timeout & Retry Strategy

## Objective
Keep jobs moving when drivers reject, ignore or lose connectivity.

## Timeout Categories
```text
candidate evaluation timeout
offer timeout
reservation timeout
driver response timeout
dispatch cycle timeout
```

All are configurable.

## Retry Strategy
Do not retry infinitely.

Example:
```text
Attempt 1 -> nearest eligible ring
Attempt 2 -> expanded radius
Attempt 3 -> broader strategy
Attempt 4 -> operational escalation
```

## Backoff
Use short bounded delays between attempts.

Avoid tight loops that overload Redis/NATS/database.

## Supply Expansion
When supply is scarce:
```text
increase radius
relax soft preferences
expand eligible vehicle classes only if business rules allow
```

Never relax hard safety/legal constraints.

## Scheduled Jobs
Scheduled jobs should not continuously dispatch too early.

Use a planning window:
```text
scheduled time
- preparation buffer
```

## No Supply
If no suitable driver exists:
- keep job searching where appropriate
- notify customer
- provide cancellation option
- escalate to operations for exceptional cargo

## Definition of Done
Retries are bounded, observable and do not violate eligibility constraints.
