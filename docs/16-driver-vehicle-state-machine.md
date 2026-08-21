# Driver & Vehicle State Machine

## Driver

```text
OFFLINE -> AVAILABLE -> OFFERED -> ACCEPTED -> ON_TRIP -> AVAILABLE
```

Other states: `PAUSED`, `SUSPENDED`, `BLOCKED`.

A driver cannot accept jobs with expired required documents, no verified active vehicle, or stale location.

## Vehicle

```text
PENDING_VERIFICATION -> VERIFIED
                         |-> SUSPENDED
                         |-> EXPIRED
```

Vehicle capabilities determine job eligibility. Capacity and service requirements must also pass.

Location records include timestamp, accuracy, speed and heading. Dispatch excludes stale locations.
