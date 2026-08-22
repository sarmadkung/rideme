# 181 — Performance, Load & Stress Testing

## Objective
Establish measurable performance limits before production scale.

## Workloads
Test:
- API requests
- booking bursts
- dispatch
- location updates
- realtime connections
- chat
- notifications
- queue processing

## Scenarios
```text
Normal
Peak
Burst
Sustained
Failure/Recovery
```

## Metrics
- p50
- p95
- p99
- throughput
- error rate
- CPU
- memory
- database latency
- queue lag

## Principle
Load tests should represent realistic marketplace behavior, not just synthetic HTTP traffic.

## Definition of Done
Known capacity limits exist for critical services and bottlenecks are documented.
