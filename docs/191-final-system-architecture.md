# 191 — Final System Architecture

## Objective
Define the implementation-level architecture that all later documents must follow.

## Architecture
The platform uses a modular backend with React Native mobile applications and a ReactJS admin dashboard.

```text
Customer Mobile ─┐
Driver Mobile ───┤
Admin React ─────┤
                 ▼
          API / Realtime Layer
                 ▼
          Domain Application
                 │
      ┌──────────┼──────────┐
      ▼          ▼          ▼
 PostgreSQL    Redis      Events
 + PostGIS                 /Queue
      │          │          │
      └──────────┼──────────┘
                 ▼
          Background Workers
```

## Core domains
- Identity
- Users
- Vehicles
- Services
- Pricing
- Booking
- Orders
- Jobs
- Dispatch
- Tracking
- Payments
- Ledger
- Merchant
- Delivery
- Cargo
- Notifications
- Chat
- Safety
- Fraud
- Support
- Admin
- Analytics

## Architecture rules
1. PostgreSQL is authoritative for transactional state.
2. Redis is not the source of truth for critical business state.
3. Critical mutations are idempotent.
4. Domain events are asynchronous where appropriate.
5. Business logic is server-side.
6. Mobile business logic remains shared where possible.
7. Native modules are isolated behind interfaces.
8. Services are extracted only when operationally justified.
9. Financial records are append-oriented and auditable.
10. Every production-critical workflow is observable.

## Agent tasks
- Establish module boundaries.
- Document dependencies.
- Identify synchronous versus asynchronous flows.
- Ensure all subsequent implementation documents follow this architecture.

## Acceptance criteria
- Every major domain has an owner/module.
- No circular domain dependency is introduced.
- Critical state has a clear source of truth.
- Architecture supports rides, delivery, grocery, and cargo without separate platforms.
