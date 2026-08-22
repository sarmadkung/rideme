# 144 — Admin Configuration & Feature Flags

## Objective
Manage controlled product behavior through configuration and feature flags.

## Flag Scope
Possible scopes:
```text
GLOBAL
REGION
CITY
SERVICE
USER_SEGMENT
MERCHANT
```

## Examples
```text
new_checkout
new_dispatch_algorithm
truck_service
cash_payment
scheduled_delivery
```

## Safety
Critical business behavior should not depend on an uncontrolled client-side flag.

Server-side evaluation is authoritative.

## Rollout
Support:
```text
OFF
INTERNAL
PERCENTAGE
TARGETED
FULL
```

## Definition of Done
Features can be safely rolled out, monitored and rolled back.
