# Database Schema

## Core Tables

### users
`id, phone, email, name, avatar_url, status, created_at, updated_at`

### drivers
`id, user_id, verification_status, rating, completion_rate, cancellation_rate, acceptance_rate`

### vehicles
`id, owner_user_id, type, make, model, year, plate_number, capacity_kg, dimensions, verification_status`

### vehicle_capabilities
`vehicle_id, capability`

### driver_vehicles
`driver_id, vehicle_id, is_primary, status`

### jobs
`id, type, requester_user_id, merchant_id, status, scheduled_at, pricing_quote_id, assigned_driver_id, assigned_vehicle_id`

### job_stops
`id, job_id, sequence, type, latitude, longitude, address, contact_name, contact_phone`

### assignments
`id, job_id, driver_id, vehicle_id, status, offered_at, accepted_at, completed_at`

### driver_locations
`driver_id, vehicle_id, latitude, longitude, accuracy, heading, speed, recorded_at`

### payments / ledger_entries
Use an append-only financial ledger.

### merchants / merchant_orders
Merchant accounts and external order references.

### support_cases / audit_events
Operational support and immutable audit history.

## Rules
- PostgreSQL is the durable source of truth.
- PostGIS stores geographic data.
- Redis holds current/ephemeral driver state.
- Financial records are immutable.
- Important state transitions are audited.
- Use UTC timestamps.
