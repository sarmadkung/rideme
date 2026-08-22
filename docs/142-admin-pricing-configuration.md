# 142 — Admin Pricing Configuration

## Objective
Allow authorized staff to configure service pricing without code deployment.

## Configuration
Examples:
```text
base fare
per km
per minute
minimum fare
vehicle modifier
cargo modifier
waiting rate
scheduled surcharge
peak multiplier
zone surcharge
discount
```

## Versioning
Every pricing configuration has:
```text
version
effective_from
created_by
status
```

## Activation
Do not overwrite active pricing.

Create a new version and activate it at a controlled time.

## Testing
Admin should preview:
```text
service
distance
duration
vehicle
cargo
zone
→ estimated price
```

## Definition of Done
Pricing changes are versioned, auditable and reversible.
