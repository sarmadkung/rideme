# 175 — Cost, Capacity & Infrastructure Performance

## Objective
Control AWS and infrastructure cost while preserving service quality.

## Cost Categories
- ECS/compute
- database
- Redis
- object storage
- networking
- map providers
- messaging
- observability

## Capacity Planning
Track:
```text
requests/sec
active users
active drivers
realtime connections
queue throughput
database connections
storage growth
```

## Scaling
Prefer measured autoscaling over premature over-provisioning.

## Cost Controls
- resource tagging
- budgets
- alerts
- right-sizing
- lifecycle policies
- cache strategy

## Definition of Done
Capacity and cost are continuously measurable and have owners/thresholds.
