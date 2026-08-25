# 192 — Database Architecture

## Objective
Define the PostgreSQL/PostGIS persistence architecture.

## Primary database
PostgreSQL is the authoritative transactional database.

PostGIS is used for geographic data and queries.

## Major schema groups

### Identity
- users
- user_profiles
- roles
- permissions
- user_roles
- sessions

### Vehicles
- vehicles
- vehicle_types
- vehicle_capabilities
- vehicle_documents
- driver_vehicle_assignments

### Services
- service_types
- service_capabilities
- service_zones
- provider_service_eligibility

### Marketplace
- quotes
- bookings
- orders
- jobs
- job_stops
- job_assignments

### Location
- locations
- tracking_sessions
- location_events
- geofences

### Commerce
- merchants
- stores
- products
- carts
- order_items

### Financial
- payment_intents
- payment_transactions
- ledger_accounts
- ledger_entries
- refunds
- settlements
- payout_records

### Communication
- conversations
- conversation_participants
- messages
- notifications

### Trust
- ratings
- incidents
- fraud_cases
- verification_reviews

### Operations
- support_tickets
- audit_logs
- feature_flags
- configuration

## Data principles
- Use UUIDs for externally exposed identifiers.
- Use database constraints for invariants.
- Use timestamps consistently in UTC.
- Add indexes based on query patterns.
- Use PostGIS geometry/geography types where appropriate.
- Avoid storing derived data when it can be safely computed.
- Store immutable financial events separately from mutable operational state.

## Geographic requirements
Support:
- pickup/destination points
- service zones
- driver proximity
- geofences
- route-related metadata

## Agent tasks
- Produce ERD.
- Define tables, columns, types, constraints.
- Define indexes.
- Define foreign keys.
- Define migration ordering.
- Identify high-volume tables and retention strategy.

## Acceptance criteria
All transactional entities have a clear ownership domain and source of truth.
