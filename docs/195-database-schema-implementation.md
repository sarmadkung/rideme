# 195 — Database Schema Implementation

## Objective
Turn the database architecture into concrete PostgreSQL migrations.

## Required output
Create migrations for:
1. extensions
2. identity
3. users/roles
4. vehicles
5. services
6. pricing foundations
7. bookings/orders/jobs
8. locations/tracking
9. merchants/products
10. payments/ledger
11. communication
12. safety/fraud
13. support/audit

## Rules
- Every migration is deterministic.
- Never modify production schema manually.
- Add constraints for state integrity.
- Index actual access paths.
- Avoid premature indexes on low-volume tables.
- Use UTC timestamps.
- Use soft deletion only where business requirements need it.

## Spatial
Enable PostGIS and use appropriate spatial indexes.

## Agent tasks
- Generate migration files.
- Generate schema diagrams.
- Create seed strategy.
- Add migration CI validation.

## Acceptance criteria
Fresh database creation succeeds from zero using migrations only.
