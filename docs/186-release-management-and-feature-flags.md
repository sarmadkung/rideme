# 186 — Release Management & Feature Flags

## Objective
Release changes safely and reversibly.

## Release Flow
```text
Develop
 → PR
 → CI
 → Staging
 → Smoke Tests
 → Canary/Controlled Rollout
 → Production
```

## Feature Flags
Use flags for risky changes:
- new pricing
- dispatch algorithm
- new checkout
- new map provider
- new service type

## Rollback
Rollback can mean:
- previous deployment
- feature flag OFF
- provider fallback
- configuration rollback

## Definition of Done
High-risk releases have a reversible rollout strategy.
