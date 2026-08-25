# Logistics Platform

A unified mobility, delivery, grocery, and cargo platform connecting customers, drivers, merchants, fleets, and operations through one ecosystem.

## Product Vision

The platform lets providers register motorcycles, rickshaws, cars, loaders, vans, and trucks and participate in eligible services according to vehicle capability, verification, geography, and operational rules.

### Core services

- Ride booking
- Ride sharing
- Grocery delivery
- Parcel delivery
- Multi-stop delivery
- Cargo and loader services
- Truck/commercial transport
- Scheduled rides and deliveries
- Merchant fulfillment
- Real-time dispatch and tracking
- Payments and settlements
- Customer, driver, merchant, and support communication

## Actors

### Customer
Registration, ride/delivery booking, grocery ordering, tracking, payment, ratings, safety, and support.

### Driver / Provider
Registration, vehicle registration, verification, availability, job acceptance, navigation, completion, earnings, communication, and incident reporting.

### Merchant
Store management, catalog, order fulfillment, delivery coordination, settlements, analytics, and support.

### Operations
Driver/vehicle approval, merchant management, live dispatch, pricing, service zones, incidents, fraud/risk, support, configuration, and audit.

## Vehicle Model

Vehicles are capability-driven rather than hard-coded to one service.

| Vehicle | Possible Services |
|---|---|
| Motorcycle | Ride, parcel, grocery |
| Rickshaw | Ride, shared ride, selected delivery |
| Car | Ride, shared ride, selected delivery |
| Loader | Cargo, heavy delivery |
| Van | Delivery, grocery, cargo |
| Truck | Cargo, commercial transport |

Eligibility depends on vehicle type, capacity, documentation, verification, geography, service rules, and operational configuration.

---

# Technology Stack

## Mobile

**React Native + TypeScript**

Used for customer and driver applications.

React Native is the default because it provides cross-platform efficiency while allowing native modules for platform-specific capabilities or performance-sensitive functionality.

Native code should be isolated behind shared interfaces. Do not duplicate complete screens and business logic for iOS and Android merely because a feature contains native code.

Typical native-module candidates:

- background location
- advanced location tracking
- navigation integrations
- push notification infrastructure
- device-specific capabilities
- performance-sensitive native functionality

## Admin Web

**ReactJS + TypeScript**

The Admin/Operations dashboard is a React SPA.

It covers dispatch, drivers, vehicles, merchants, pricing, zones, support, incidents, analytics, configuration, and audit.

**Next.js is not the Admin Dashboard framework.**

## Backend

Primary:

- Node.js
- TypeScript

Performance/system-oriented services where justified:

- Go
- Rust

Multiple languages should only be introduced when there is a measurable technical reason.

## APIs

Depending on domain requirements:

- REST
- GraphQL
- gRPC
- WebSocket/realtime protocols

## Data

- PostgreSQL
- PostGIS
- Redis
- Object storage
- Event/queue infrastructure

## Infrastructure

- AWS
- Docker
- ECS
- GitHub Actions
- Infrastructure as Code
- Centralized logs
- Metrics
- Distributed tracing
- Monitoring and alerting

---

# Architecture Principles

## Modular first

Core domains remain independently understandable and testable:

```text
Identity
Vehicles
Services
Pricing
Booking
Orders
Jobs
Dispatch
Tracking
Payments
Ledger
Merchant
Delivery
Cargo
Safety
Fraud
Notifications
Chat
Support
Admin
Analytics
Infrastructure
```

Start with clear modules and extract independent services only when scale, ownership, reliability isolation, or performance justifies it.

## Source of truth

Transactional systems are authoritative.

- PostgreSQL → transactional state
- Ledger → financial truth
- Domain services → business state
- Redis → ephemeral/accelerated state
- Analytics storage → analytical representation

## Event-driven integration

```text
Domain Event
    ↓
Event Bus / Queue
    ├── Notifications
    ├── Analytics
    ├── Merchant
    ├── Dispatch
    └── Support
```

Consumers must be idempotent.

## Server authoritative

The server is authoritative for:

- pricing
- payment
- assignment
- eligibility
- permissions
- service state

Realtime messages and client state are synchronization mechanisms, not sources of truth.

---

# High-Level Architecture

```text
Customers ───────┐
Drivers ─────────┤
Merchants ───────┤
Admin React ─────┤
                 ▼
          API / Realtime Gateway
                 │
     ┌───────────┼───────────┐
     ▼           ▼           ▼
 Booking      Dispatch    Payments
 Orders       Tracking    Ledger
     │           │           │
     └───────────┼───────────┘
                 ▼
          Events / Queues
                 │
       ┌─────────┼─────────┐
       ▼         ▼         ▼
 PostgreSQL    Redis    Object Storage
   + PostGIS
                 │
                 ▼
          Analytics / BI
```

---

# Recommended Repository

Use a monorepo.

```text
/
├── README.md
├── docs/
│   ├── 028-...
│   ├── 029-...
│   └── 190-...
│
├── apps/
│   ├── customer-mobile/
│   ├── driver-mobile/
│   └── admin-web/
│
├── services/
│   ├── api/
│   ├── auth/
│   ├── booking/
│   ├── dispatch/
│   ├── tracking/
│   ├── payments/
│   ├── merchant/
│   ├── delivery/
│   ├── cargo/
│   ├── notifications/
│   ├── chat/
│   ├── support/
│   ├── safety/
│   └── analytics/
│
├── packages/
│   ├── ui/
│   ├── types/
│   ├── validation/
│   ├── api-client/
│   ├── config/
│   └── domain/
│
├── infrastructure/
│   ├── aws/
│   ├── docker/
│   └── ci/
│
└── scripts/
```

This is the target structure; actual service extraction should follow proven domain boundaries rather than creating microservices prematurely.

---

# Documentation Index

The detailed documentation is organized into numbered documents.

| Documents | Area |
|---|---|
| 028–032 | Identity & Vehicles |
| 033–037 | Jobs, Quotes & Booking |
| 038–050 | Dispatch & Realtime |
| 051–064 | Payments & Financials |
| 065–078 | Merchant & Grocery |
| 079–092 | Parcel, Delivery & Cargo |
| 093–106 | Maps, Navigation & Tracking |
| 107–120 | Safety, Fraud & Trust |
| 121–134 | Notifications, Chat & Support |
| 135–148 | Admin & Operations |
| 149–162 | Analytics & BI |
| 163–176 | Infrastructure, AWS & DevOps |
| 177–190 | Testing, Performance & Production |

## 028–032 — Identity & Vehicles

Identity, authentication, users, driver onboarding, vehicle registration, vehicle capabilities, and verification.

## 033–037 — Jobs, Quotes & Booking

Service requests, quotes, booking, job creation, job lifecycle, and booking states.

## 038–050 — Dispatch & Realtime

Driver availability, matching, assignment, realtime communication, location updates, and job synchronization.

## 051–064 — Payments & Financials

Payment methods, pricing, commissions, refunds, wallet/credits where applicable, ledger, settlements, and reconciliation.

## 065–078 — Merchant & Grocery

Merchant onboarding, stores, catalogs, grocery orders, fulfillment, delivery integration, and settlements.

## 079–092 — Parcel, Delivery & Cargo

Parcel delivery, pickup/drop-off, proof of delivery, multi-stop delivery, loaders, trucks, and commercial transportation.

## 093–106 — Maps, Navigation & Tracking

Geolocation, maps, routing, navigation, ETA, geofencing, live tracking, and location architecture.

## 107–120 — Safety, Fraud & Trust

Verification, SOS, trusted contacts, ratings, fraud detection, abuse prevention, incidents, safety audit, device/session trust, and safety operations.

## 121–134 — Notifications, Chat & Support

Push, SMS, email, OTP, preferences, localization, events, chat, chat privacy, support cases, support routing, operational support actions, and communication observability.

## 135–148 — Admin & Operations

RBAC, driver/vehicle operations, merchant management, order operations, dispatch console, pricing, service zones, feature flags, review queues, audit, and React admin architecture.

## 149–162 — Analytics & BI

Event taxonomy, product analytics, marketplace analytics, driver/merchant analytics, unit economics, KPIs, data warehouse, analytics models, dashboards, experimentation, governance, and data quality.

## 163–176 — Infrastructure & DevOps

AWS, networking, security, Docker, ECS, PostgreSQL/PostGIS, Redis, queues, GitHub Actions, secrets, observability, monitoring, backups, disaster recovery, capacity, and cost.

## 177–190 — Testing & Production

Testing strategy, backend/frontend testing, E2E, load testing, mobile performance, security, offline resilience, idempotency, release management, production readiness, incident response, and launch.

---

# Development Roadmap

```text
Phase 1   Identity & Vehicles
Phase 2   Jobs, Quotes & Booking
Phase 3   Dispatch & Realtime
Phase 4   Payments & Financials
Phase 5   Merchant & Grocery
Phase 6   Parcel, Delivery & Cargo
Phase 7   Maps & Tracking
Phase 8   Safety, Fraud & Trust
Phase 9   Notifications, Chat & Support
Phase 10  Admin & Operations
Phase 11  Analytics & BI
Phase 12  Infrastructure & DevOps
Phase 13  Testing & Production
```

These are capability phases, not necessarily strict sequential development gates.

---

# MVP Recommendation

Do not launch every service simultaneously.

Start with a narrow geography and a small number of services.

## Customer

- registration/login
- location selection
- ride booking
- delivery booking
- tracking
- payment
- ratings
- support

## Driver

- registration
- vehicle registration
- verification
- online/offline
- job acceptance
- navigation
- completion
- earnings

## Operations

- driver approval
- vehicle approval
- live jobs
- dispatch
- pricing
- service zones
- support
- incidents

## Backend

- identity
- vehicles
- booking
- dispatch
- tracking
- payments
- notifications
- support

Grocery marketplace, advanced cargo, complex fleet management, experimentation, and advanced BI can be introduced progressively.

---

# Realtime & Location

Realtime is critical for:

- driver locations
- job assignment
- ride state
- delivery tracking
- chat
- dispatch
- operations

Use:

```text
Client
 ↓
Realtime Gateway
 ↓
Event / State Layer
 ↓
Domain Services
```

After reconnect, app restart, or missed events, clients must reconcile against authoritative server state.

Driver location architecture must account for:

- foreground/background location
- battery consumption
- permissions
- unstable connectivity
- location update frequency
- active-job tracking
- privacy and retention

---

# Financial Architecture

Financial state requires stronger consistency than ordinary CRUD.

Core concepts:

```text
Customer Charge
Provider Earnings
Platform Fee
Commission
Refund
Adjustment
Settlement
Ledger
```

The ledger should be treated as immutable in principle. Corrections should be represented through new transactions/adjustments rather than silently rewriting historical financial records.

Analytics consumes financial data but does not become the financial source of truth.

---

# Security & Privacy

Security controls include:

- authentication
- server-side authorization
- RBAC
- scoped admin access
- rate limiting
- fraud detection
- secure file handling
- secrets management
- audit logs
- encryption in transit
- appropriate encryption at rest
- dependency/security scanning

Sensitive data may include:

- location
- identity documents
- contact information
- payment information
- conversations
- delivery addresses

Principles:

1. Collect only what is necessary.
2. Restrict access by role.
3. Do not expose private contact details unnecessarily.
4. Define retention periods.
5. Audit sensitive access.
6. Remove/anonymize data where appropriate.
7. Do not copy sensitive data into analytics without a defined purpose.

---

# Observability

Important requests/events should be traceable using identifiers such as:

```text
request_id
correlation_id
trace_id
event_id
job_id
order_id
payment_id
notification_id
```

Monitor:

- API latency/errors
- database latency
- queue depth
- dispatch latency
- assignment success
- realtime connections
- notification delivery
- payment failures
- crash rates
- support SLA
- infrastructure health

---

# Testing Strategy

Use a balanced pyramid:

```text
             E2E
       Integration / Contract
          Unit / Component
```

Critical areas:

- pricing
- booking
- dispatch
- payments
- state transitions
- authorization
- idempotency
- notifications
- tracking
- support
- safety

Use Playwright for web/admin E2E where appropriate and an appropriate mobile E2E strategy for React Native.

---

# Production Engineering

Production should support:

- Dockerized services
- AWS
- ECS
- secure networking
- PostgreSQL/PostGIS
- Redis
- queues/workers
- CI/CD
- secrets management
- backups
- disaster recovery
- monitoring
- alerting
- runbooks
- staged releases
- rollback

---

# Engineering Principles

1. **Mobile-first operational thinking** — the driver app is mission-critical.
2. **Server authoritative** — never trust client state for business-critical decisions.
3. **Idempotency** — critical mutations must be safe to retry.
4. **Realtime is not truth** — realtime accelerates synchronization.
5. **Poor connectivity is normal** — design for unreliable networks.
6. **Measure before optimizing** — use native modules, Go, Rust, or microservices when justified by evidence.
7. **Avoid premature microservices** — modular architecture first.
8. **Financial correctness** — ledger correctness has priority over convenience.
9. **Security by default** — authorization is enforced server-side.
10. **Operational simplicity** — the system must remain understandable and operable.

---

# Development Workflow

```text
Issue
 ↓
Technical Design
 ↓
Implementation
 ↓
Unit / Integration Tests
 ↓
Pull Request
 ↓
CI
 ↓
Review
 ↓
Staging
 ↓
E2E / Smoke Tests
 ↓
Production
 ↓
Monitoring
```

Significant architectural decisions should be documented.

---

# Definition of Done

A production-ready feature normally includes:

- domain logic
- API
- authorization
- validation
- error handling
- loading/empty/error states
- observability
- analytics events where applicable
- unit/integration tests
- E2E coverage where critical
- mobile/offline handling where applicable
- audit requirements where applicable
- documentation
- operational support

---

# Documentation Rules

This README is the **single entry point** to the project documentation.

Detailed documents live under `docs/`.

Use:

```text
028-document-name.md
029-document-name.md
...
190-document-name.md
```

Do **not** create another `README.md` for the documentation series.

## Source of Truth

When documents conflict:

1. Explicitly approved architectural decisions take precedence.
2. Later approved decisions supersede earlier proposals.
3. Implementation must be updated to match the latest approved architecture.
4. Major changes should be recorded as architecture decisions.

---

# Current Architecture Decisions

| Area | Decision |
|---|---|
| Mobile | React Native |
| Mobile language | TypeScript |
| Admin | ReactJS |
| Admin language | TypeScript |
| Backend primary | Node.js + TypeScript |
| High-performance backend | Go/Rust where justified |
| Primary database | PostgreSQL |
| Geospatial | PostGIS |
| Cache/realtime state | Redis |
| Containers | Docker |
| Compute | AWS ECS |
| CI/CD | GitHub Actions |
| Realtime | WebSocket/event-driven architecture |
| Analytics | Separate analytics storage |
| Web E2E | Playwright |
| Architecture | Modular first; extract services when justified |

---

# What Comes Next

The documentation phase is now complete through document 190.

The next work should be implementation-oriented rather than creating more generic documentation.

## 1. Reconcile the documentation

Review documents 028–190 together and resolve:

- conflicting requirements
- duplicate concepts
- inconsistent terminology
- overlapping services
- missing dependencies
- architecture decisions requiring final approval

## 2. Finalize implementation architecture

Define:

```text
Apps
 ↓
API
 ↓
Domain Modules
 ↓
Database
 ↓
Redis
 ↓
Events
 ↓
Workers
```

## 3. Finalize database design

Define:

- PostgreSQL schema
- PostGIS schema
- indexes
- relationships
- migrations
- state machines
- constraints

## 4. Finalize API contracts

Define contracts for:

- authentication
- vehicles
- services
- pricing
- booking
- dispatch
- tracking
- payments
- merchant
- delivery
- cargo
- support

## 5. Initialize the monorepo

Set up:

- workspace
- customer mobile
- driver mobile
- React admin
- backend
- shared packages
- linting
- formatting
- testing
- CI

## 6. Implement the MVP

Recommended order:

```text
Identity
 ↓
Vehicles
 ↓
Service Eligibility
 ↓
Booking
 ↓
Pricing
 ↓
Dispatch
 ↓
Tracking
 ↓
Payment
 ↓
Completion
```

Then progressively add grocery, parcel, cargo, merchant, advanced operations, analytics, and additional services.

---

# Target Platform

```text
                 LOGISTICS PLATFORM
                         │
        ┌────────────────┼────────────────┐
        │                │                │
      RIDES          DELIVERY          CARGO
        │                │                │
   ┌────┴────┐      ┌────┴────┐      ┌────┴────┐
   │ Car     │      │ Grocery │      │ Loader  │
   │ Rickshaw│      │ Parcel  │      │ Van     │
   │ Shared  │      │ Multi   │      │ Truck   │
   └─────────┘      └─────────┘      └─────────┘
                         │
                ┌────────┴────────┐
                │                 │
             MERCHANT          CUSTOMER
                │                 │
                └────────┬────────┘
                         │
                    DRIVER/FLEET
                         │
                  DISPATCH + MAPS
                         │
              PAYMENTS + LEDGER
                         │
             SAFETY + FRAUD + TRUST
                         │
              SUPPORT + ANALYTICS
                         │
                 AWS PLATFORM
```

The objective is not simply to build another ride-hailing or delivery application. It is to build a **general-purpose mobility and logistics platform** in which different vehicle types and services share the same identity, vehicle, booking, dispatch, tracking, payment, safety, and operations foundation.

---

# Status

**Documentation:** Complete through document 190  
**Architecture:** Defined at high level  
**Technology stack:** Defined  
**Mobile:** React Native  
**Admin:** ReactJS  
**Backend:** Node.js/TypeScript with Go/Rust where justified  
**Infrastructure:** AWS + Docker + ECS  
**Next milestone:** Documentation reconciliation → database/API design → monorepo implementation → MVP

---

# License

License and commercial terms should be defined before public distribution or third-party contribution.
