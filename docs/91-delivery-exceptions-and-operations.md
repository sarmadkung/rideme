# 91 — Delivery Exceptions & Operations

## Objective
Create a structured exception system for real-world delivery failures.

## Exception Types
```text
NO_DRIVER
DRIVER_CANCELLED
MERCHANT_DELAY
CUSTOMER_UNAVAILABLE
WRONG_ADDRESS
VEHICLE_FAILURE
PACKAGE_DAMAGE
PAYMENT_ISSUE
SAFETY_ISSUE
```

## Severity
```text
INFO
WARNING
HIGH
CRITICAL
```

## Exception Lifecycle
```text
OPEN
ACKNOWLEDGED
IN_PROGRESS
RESOLVED
CLOSED
```

## Operations Actions
Depending on exception:
- reassign
- contact customer
- contact merchant
- reschedule
- return package
- refund
- escalate

## Audit
Every manual intervention requires actor, reason and timestamp.

## Definition of Done
Operations can manage abnormal deliveries without editing production data directly.
