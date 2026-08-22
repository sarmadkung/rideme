# 38 — Dispatch Engine Architecture

## Objective
Design the system that converts an active job into a safe, efficient driver/vehicle assignment.

## Core Flow
```text
Job SEARCHING
      ↓
Dispatch Trigger
      ↓
Candidate Discovery
      ↓
Eligibility Filtering
      ↓
Scoring
      ↓
Reservation
      ↓
Driver Offer
      ↓
Accept / Reject / Timeout
      ↓
Assignment
      ↓
Realtime Notification
```

## Responsibilities
Dispatch owns:
- candidate discovery
- eligibility
- scoring
- offer creation
- reservation
- timeout
- retry
- reassignment

Dispatch does not own:
- pricing
- payment
- user authentication
- driver document verification

## Architecture
Start as a module in the Go backend.

Use:
- PostgreSQL/PostGIS for durable data
- Redis for fast operational state and geo lookup
- NATS for asynchronous events
- WebSocket gateway for client delivery

Do not split dispatch into a separate microservice until load or team boundaries justify it.

## Dispatch Job
Conceptually:
```text
DispatchAttempt
├── job_id
├── attempt_number
├── strategy_version
├── candidates
├── selected_candidate
├── status
├── started_at
└── completed_at
```

## Safety Principle
A driver must never be considered available merely because the client says so.

The backend validates authoritative driver state before reservation.

## Definition of Done
- architecture supports all initial job types
- dispatch can be retried safely
- reservation prevents double assignment
- every attempt is observable
