# 102 — Location Privacy & Security

## Objective
Protect customer and driver location data.

## Principles
- collect only what is needed
- limit access
- minimize retention
- encrypt transport
- audit privileged access

## Driver Location
Customers should see only location needed for active service according to product policy.

Do not expose:
- driver's private address
- historical route unrelated to their job
- unnecessary personal information

## Access Control
Examples:
```text
Customer → own active job
Driver → assigned jobs
Merchant → relevant delivery
Operations → authorized operational scope
```

## Retention
Separate:
```text
live operational location
historical analytics
audit/security records
```

They should not automatically share the same retention period.

## Definition of Done
Unauthorized users cannot access location streams or historical location data.
