# 168 — Redis Cache & Realtime State

## Objective
Use Redis for low-latency ephemeral and coordination workloads.

## Suitable Uses
- driver current location
- availability state
- short-lived cache
- rate limiting
- distributed locks where justified
- realtime presence
- job queues where architecture permits

## Important
Redis should not silently become the only source of truth for critical business records.

## TTL
Every ephemeral key should have an intentional TTL unless it is explicitly persistent by design.

## Failure
Applications must define behavior when Redis is unavailable.

## Definition of Done
Redis improves performance without compromising transactional correctness.
