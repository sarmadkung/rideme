# 26 — Database Implementation

## Objective

Turn the conceptual database model into migrations, constraints and indexes that can support the MVP.

## Database

PostgreSQL + PostGIS.

Use migrations as the source of truth.

## Core Migration Order

```text
001_extensions
002_users
003_roles
004_drivers
005_driver_documents
006_vehicles
007_vehicle_capabilities
008_driver_vehicles
009_jobs
010_job_stops
011_job_requirements
012_assignments
013_quotes
014_payments
015_ledger
016_ratings
017_merchants
018_merchant_orders
019_support
020_audit_events
```

Exact numbering may change during implementation.

## Geographic Data

Use PostGIS geometry/geography types for:
- pickup
- destination
- zones
- driver location where durable storage is required

Use appropriate spatial indexes.

## Important Indexes

Examples:

```text
users(phone)
drivers(user_id)
vehicles(plate_number)
driver_vehicles(driver_id, status)
jobs(status, created_at)
jobs(assigned_driver_id, status)
job_stops(job_id, sequence)
assignments(job_id, status)
merchant_orders(merchant_id, status)
```

Add spatial indexes for geographic queries.

## Uniqueness

Examples:
- normalized phone
- vehicle plate number where applicable
- external merchant order ID scoped to merchant
- role/user relationship

## State Constraints

Where practical, use database constraints to prevent impossible data.

Do not rely only on frontend validation.

## Financial Integrity

Ledger entries must be immutable.

Use:
- transaction boundaries
- foreign keys
- positive/negative amount conventions
- reference IDs
- idempotency keys

## Timestamps

Store timestamps in UTC.

Prefer:

```text
created_at
updated_at
```

and explicit event timestamps where needed.

## Soft Delete

Do not blindly add `deleted_at` to every table.

Use soft deletion only where business/audit requirements need it.

Financial and audit records should not be deleted.

## Driver Location

Do not make the primary relational database the high-frequency realtime location store.

Use Redis for current state and a separate durable strategy for historical tracking.

## Seeds

Seed:
- roles
- sample zones
- test users
- test vehicles
- capabilities

Seed scripts must be repeatable.

## Migration Rules

- migrations are forward-only in shared environments
- every migration is reviewed
- destructive migrations require explicit rollout planning
- production schema changes should be backward-compatible where possible
