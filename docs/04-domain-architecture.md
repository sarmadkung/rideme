# Domain & Technical Architecture

## Architectural Principle

Start as a modular monolith. Keep domain boundaries clean so services can be extracted later.

## Recommended Stack

### Mobile
React Native + Expo

### Web
Next.js

### Backend
Go

### Database
PostgreSQL + PostGIS

### Cache / ephemeral state
Redis

### Realtime
WebSockets

### Messaging
NATS initially

### Storage
S3-compatible object storage

### Observability
Sentry + OpenTelemetry

## Core Domains

```text
identity
users
drivers
vehicles
documents
jobs
dispatch
pricing
payments
wallet
ratings
support
merchants
notifications
zones
fraud
analytics
```

## Core Entity Model

```text
User
  ├── Driver
  │     └── DriverVehicle
  │             └── Vehicle
  │                    └── VehicleCapability
  │
  ├── Customer
  └── Merchant

Job
  ├── JobStop[]
  ├── JobRequirement
  ├── PricingQuote
  ├── Assignment
  ├── Payment
  └── Proof[]

Assignment
  ├── Driver
  └── Vehicle
```

## Job Abstraction

All operational work should be represented as a Job.

Job types:
- RIDE
- PARCEL
- GROCERY
- CARGO
- FREIGHT

Common fields:
- requester
- pickup
- destination
- stops
- scheduled_at
- requirements
- pricing
- status
- assigned driver
- assigned vehicle
- payment
- audit history

## Vehicle Capability Model

A vehicle should have capabilities rather than being hard-coded to one service.

Example:

```text
Motorcycle
  passenger=true
  parcel=true
  grocery=true
  heavy_cargo=false

Suzuki
  passenger=false
  parcel=true
  grocery=true
  small_cargo=true
  heavy_cargo=false
```

## Dispatch

Candidate score should consider:

- ETA
- distance
- vehicle suitability
- capacity
- capability
- driver reliability
- customer requirements
- price
- route compatibility
- destination demand
- empty-km penalty
- future-job opportunity

## Future Extraction Boundaries

Potential future services:
- Dispatch Service
- Pricing Service
- Payments Service
- Notification Service
- Tracking Service
- Merchant API

Do not split them into microservices until scale or team boundaries justify it.
