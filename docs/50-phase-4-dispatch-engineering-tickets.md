# 50 — Phase 4 Dispatch Engineering Tickets

## Goal
Build reliable driver matching and assignment for active jobs.

## DSP-001 — Dispatch Module
Create dispatch domain/application module.

Acceptance: dispatch can be invoked by a job-searching event.

## DSP-002 — Candidate Discovery
Implement Redis geo lookup plus backend eligibility filtering.

Acceptance: only fresh, eligible drivers reach scoring.

## DSP-003 — Capability Matching
Implement hard constraints for vehicle/service requirements.

Acceptance: impossible vehicle matches are rejected.

## DSP-004 — Scoring Engine
Implement versioned candidate scoring.

Acceptance: ranking is deterministic for identical inputs.

## DSP-005 — Dispatch Strategy Configuration
Support configurable weights/radii/timeouts.

Acceptance: strategy can change without code deployment.

## DSP-006 — Driver Reservation
Implement atomic reservation.

Acceptance: one driver cannot be reserved by two conflicting jobs.

## DSP-007 — Offer Lifecycle
Implement offer create/accept/reject/expire.

Acceptance: expired offers cannot be accepted.

## DSP-008 — Dispatch Worker
Consume job-searching events and run dispatch attempts.

Acceptance: duplicate events do not create duplicate assignments.

## DSP-009 — Timeout Worker
Expire offers and reservations.

Acceptance: abandoned offers release resources automatically.

## DSP-010 — Reassignment
Return failed assignments to searching.

Acceptance: repeated failures are bounded.

## DSP-011 — Concurrency Tests
Run parallel assignment attempts.

Acceptance:
```text
1 assignment winner
0 double assignments
```

## DSP-012 — WebSocket Gateway
Implement authenticated realtime subscriptions.

Acceptance: job and driver clients receive authorized updates.

## DSP-013 — Location Pipeline
Integrate current driver location into Redis.

Acceptance: dispatch sees current driver state.

## DSP-014 — NATS Events
Implement versioned dispatch events and idempotent consumers.

## DSP-015 — Dispatch Observability
Track:
- time to first offer
- acceptance rate
- assignment latency
- reassignment rate
- search failure rate
- ETA prediction error

## DSP-016 — Dispatch Simulation
Create a deterministic simulator with:
- synthetic drivers
- geographic distribution
- different vehicle capabilities
- job generation
- accept/reject behavior

Use it to tune the scoring algorithm before production.

## DSP-017 — End-to-End Test
Scenario:
```text
Driver online
 -> location published
 -> customer creates job
 -> dispatch discovers driver
 -> driver receives offer
 -> accepts
 -> job assigned
 -> realtime customer update
```

## Phase 4 Exit Criteria
The platform can safely discover, rank, reserve and assign a suitable driver to a live job, recover from normal failures, and provide realtime state to customers and drivers.

Next phase: **Payments, Wallets & Driver Earnings**.
