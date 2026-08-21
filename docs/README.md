# Logistics Platform

A unified transportation and logistics marketplace that connects customers and businesses with the right vehicle for rides, deliveries, and cargo.

## Product Vision

> **Any compatible vehicle can perform multiple kinds of jobs, and the platform continuously matches demand to the best vehicle while maximizing driver profitability and minimizing empty kilometers.**

The platform is designed as a **transportation and logistics operating system**, not simply another ride-hailing application.

---

## Core Product

### Customers

Customers can:

- Book a ride
- Send a parcel/document
- Move cargo
- Schedule a delivery
- Track an active job
- Pay by supported payment methods
- Rate drivers
- Get proof of delivery

### Drivers

A driver can register one or more compatible vehicles:

- Motorcycle
- Rickshaw
- Car
- Loader
- Suzuki pickup
- Shehzore
- Mazda
- Truck

A vehicle has **capabilities**, allowing it to participate in multiple services.

For example:

```text
Suzuki Pickup
├── Parcel
├── Grocery
├── Small Cargo
└── House Moving
```

The driver does not need separate accounts or separate apps for different services.

### Merchants

Merchants can:

- Create deliveries
- Track orders
- Schedule deliveries
- Upload bulk orders
- Handle COD
- View reports
- Use the logistics API
- Manage delivery history

### Operations

Admins can:

- Monitor live jobs
- Dispatch drivers
- Verify drivers and vehicles
- Manage pricing
- Manage zones
- Handle disputes
- Monitor safety
- Manage support
- Review fraud signals
- View analytics

---

# Product Strategy

The initial product wedge is:

```text
Motorcycle + Rickshaw + Car + Loader/Suzuki
                 |
                 v
       Ride + Parcel + Cargo
```

Grocery should initially be treated as a **merchant delivery capability**, rather than a warehouse/inventory business.

Later:

```text
Ride
  ↓
Parcel
  ↓
Small Cargo
  ↓
Merchant Delivery
  ↓
COD
  ↓
Scheduled Logistics
  ↓
Return Loads
  ↓
Intercity
  ↓
Freight
  ↓
Logistics API
```

---

# Core Differentiation

The platform is built around two concepts:

## 1. Vehicle-Centric Supply

A vehicle is not permanently tied to one service.

```text
Vehicle
   ↓
Capabilities
   ↓
Eligible Jobs
```

This increases utilization and driver earning potential.

## 2. Job-Centric Demand

Customers describe what they need to move.

```text
Customer
   ↓
Job Requirements
   ↓
Vehicle Recommendation
   ↓
Pricing
   ↓
Dispatch
```

The customer should not need to understand every vehicle category.

---

# Key Competitive Advantages

The platform should compete on:

1. Multi-service vehicle utilization
2. Transparent driver earnings
3. Fair pricing
4. Route compatibility
5. Return-load matching
6. Reduced empty kilometers
7. Merchant logistics infrastructure
8. Two-sided reputation
9. Proof of delivery
10. Strong operational tooling

---

# Technology Stack

## Mobile

**React Native + Expo + TypeScript**

Two primary applications:

```text
Customer Mobile
Driver Mobile
```

React Native owns the application experience.

Native Swift/Kotlin modules are used only where platform capabilities or performance require them.

Examples:

- Background location
- GPS processing
- Biometrics
- Secure storage
- Sensors
- Specialized navigation
- Native notification capabilities

## Dashboards

**React + TypeScript + Vite**

```text
Merchant Dashboard
Admin Dashboard
```

## Public Website

**Next.js + TypeScript**

Used primarily for:

- Marketing
- SEO
- Public pages
- Documentation where appropriate

## Backend

**Go**

The Go backend handles:

- Business logic
- Jobs
- Dispatch
- Pricing
- Driver availability
- Realtime location processing
- Payments
- Merchant operations
- Notifications
- Fraud/risk processing

## Database

**PostgreSQL + PostGIS**

PostGIS is important for:

- Nearby-driver searches
- Geographic zones
- Route-related queries
- Pickup/dropoff locations
- Vehicle availability
- Geographic analytics

## Realtime / Cache

**Redis**

Used for:

- Current driver locations
- Driver availability
- Active jobs
- Geospatial availability
- Short-lived locks
- Rate limiting
- Caching

**NATS**

Used for:

- Domain events
- Asynchronous processing
- Realtime event distribution
- Background jobs

## Infrastructure

- Docker
- AWS ECS/Fargate initially
- Terraform
- GitHub Actions
- S3-compatible object storage

## Observability

- Sentry
- OpenTelemetry
- PostHog

## Testing

- Playwright for E2E
- Vitest/Jest where appropriate
- Go unit/integration tests

---

# Architecture Principle

The system follows:

```text
React Native
    ↓
Product experience / client flows

Swift + Kotlin
    ↓
Platform-specific capabilities

Go
    ↓
Business logic / dispatch / pricing / realtime

PostgreSQL + PostGIS
    ↓
Durable transactional + geographic data

Redis
    ↓
Realtime operational state

NATS
    ↓
Events / asynchronous processing
```

---

# Architecture Approach

Start with a **modular monolith**, not a microservice-heavy architecture.

Backend modules:

```text
identity/
users/
drivers/
vehicles/
documents/
jobs/
dispatch/
pricing/
payments/
wallet/
tracking/
merchants/
notifications/
support/
fraud/
analytics/
zones/
```

Services should only be extracted when there is a concrete reason such as:

- independent scaling
- team ownership
- deployment isolation
- reliability requirements
- high-volume workloads

---

# Core Domain Model

The central abstraction is the **Job**.

```text
Job
├── Ride
├── Parcel
├── Grocery
├── Cargo
└── Freight
```

Common job concepts:

- Requester
- Pickup
- Destination
- Stops
- Requirements
- Vehicle
- Driver
- Pricing
- Assignment
- Payment
- Tracking
- Proof
- Status
- Audit history

---

# Dispatch Engine

The dispatch system should **not simply select the nearest driver**.

Candidate scoring should consider:

```text
ETA
+ Vehicle suitability
+ Capacity
+ Driver reliability
+ Route compatibility
+ Price
+ Destination demand
- Empty kilometers
- Cancellation risk
```

Long term, the dispatch engine should optimize the entire network rather than individual jobs.

---

# Return-Load Marketplace

A major future differentiator is matching vehicles with return jobs.

Example:

```text
Truck

Lahore
   ↓
Islamabad

Cargo delivered
   ↓
Return to Lahore
```

Instead of returning empty:

```text
Islamabad
   ↓
Lahore

Compatible return cargo
```

This improves:

- Driver earnings
- Fleet utilization
- Customer pricing
- Platform margins
- Network efficiency

---

# Driver Economics

The platform should show estimated net earnings before accepting a job.

Example:

```text
Gross earning       Rs 650
Estimated fuel      Rs 125
Platform fee         Rs 55
---------------------------
Estimated net       Rs 470
```

Primary driver KPI:

> **Net earnings per online hour**

Supporting metrics:

- Jobs/hour
- Revenue/km
- Empty km
- Acceptance rate
- Completion rate
- Cancellation rate
- Driver retention

---

# North-Star Metric

The main business metric is:

> **Completed profitable jobs per active vehicle per day**

Other important metrics:

### Customer
- Fulfillment rate
- Pickup ETA
- Repeat rate
- Cancellation rate

### Driver
- Net earnings/hour
- Jobs/hour
- Retention
- Empty-km ratio

### Merchant
- Orders/month
- Delivery success
- COD value
- Repeat usage

### Platform
- Contribution margin/job
- CAC
- LTV
- Supply/demand ratio
- Jobs/vehicle/day

---

# Initial Launch Strategy

Do not launch nationwide.

Start with:

```text
One city
    ↓
5–8 dense connected zones
    ↓
High vehicle availability
    ↓
Reliable fulfillment
    ↓
Positive unit economics
    ↓
Expand
```

Lahore is the initial candidate city, subject to validation through local research and unit economics.

---

# Development Documentation

The documentation is organized into two major stages.

## Product & Business Documentation

```text
00-project-vision.md
01-competitive-analysis.md
02-business-model.md
03-mvp-prd.md
04-domain-architecture.md
05-dispatch-pricing.md
06-business-go-to-market.md
07-metrics-operations.md
08-roadmap.md
09-project-structure.md
10-decision-log.md
```

These define the business, product strategy, competitive positioning, MVP and initial roadmap.

## Technical & Engineering Documentation

```text
12-technical-blueprint.md
13-database-schema.md
14-api-specification.md
15-job-state-machine.md
16-driver-vehicle-state-machine.md
17-native-mobile-architecture.md
18-realtime-location-architecture.md
19-payment-wallet-settlement.md
20-auth-security.md
21-screen-map.md
22-engineering-phases.md
```

These define the technical architecture and implementation approach.

> There is intentionally no `11` document at this stage. Do not renumber existing documents just to remove the gap.

---

# Engineering Phases

The implementation roadmap is:

```text
Phase 1   Foundation
Phase 2   Identity & Vehicles
Phase 3   Jobs
Phase 4   Dispatch
Phase 5   Realtime
Phase 6   Payments
Phase 7   Operations
Phase 8   Merchant
Phase 9   Optimization
Phase 10  Scale
Phase 11  Expansion
```

Detailed requirements are documented in:

`22-engineering-phases.md`

---

# Product Evolution

Long-term architecture:

```text
                 Transportation
                       |
        +--------------+--------------+
        |              |              |
       Rides         Delivery       Cargo
        |              |              |
        +--------------+--------------+
                       |
                Merchant Logistics
                       |
                 Return Loads
                       |
                  Intercity
                       |
                    Freight
                       |
                Logistics API
                       |
          Logistics Infrastructure
```

The long-term goal is to become a **logistics infrastructure platform**, not merely a ride-hailing marketplace.

---

# Current Development Principle

Do not over-engineer before the marketplace is validated.

Priorities:

1. Supply density
2. Reliable fulfillment
3. Driver economics
4. Customer experience
5. Unit economics
6. Dispatch quality
7. Operational tooling
8. Scale

Technology should support these goals rather than become the goal itself.

---

# Current Status

The project is currently at the **product + technical blueprint stage**.

Next work should move into detailed implementation specifications and then engineering phases, beginning with the foundation.

The next documentation should continue from **23** without regenerating or replacing this README.
