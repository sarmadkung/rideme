# 136 — Admin Roles & Permissions

## Objective
Implement least-privilege access for operations staff.

## Example Roles
```text
SUPER_ADMIN
OPERATIONS_ADMIN
DISPATCHER
DRIVER_OPERATIONS
MERCHANT_OPERATIONS
FINANCE_ADMIN
SUPPORT_AGENT
TRUST_SAFETY
ANALYST
READ_ONLY
```

## Permission Format
```text
resource.action
```

Examples:
```text
driver.read
driver.suspend
pricing.update
refund.create
incident.assign
```

## Scope
Permissions may be restricted by:
- region
- city
- service
- merchant group

## Definition of Done
Every privileged admin action is checked server-side against role and scope.
