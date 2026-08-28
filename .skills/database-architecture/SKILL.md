---
name: database-architecture
description: PostgreSQL + PostGIS rules for RideMe — schema ownership, constraints as invariants, migrations, geospatial data, and the Redis boundary. Use before writing a migration, adding a table or index, or deciding whether state belongs in Postgres or Redis.
---

# Purpose

Keep transactional truth in PostgreSQL, enforced by the database rather than by hope.

# When to Use

- Any migration, table, column, constraint, or index.
- Choosing between Postgres and Redis for a piece of state.
- Geospatial queries.

# Rules

- **PostgreSQL is authoritative for transactional state.** Redis holds ephemeral and derived state only: current driver state, active job pointer, geospatial availability, short-lived locks (`docs/18`). Losing Redis must never lose business truth.
- **Invariants belong in the schema**, not only in application code: foreign keys, unique constraints, check constraints, appropriate isolation, explicit locking. Application validation is a second layer, never the only one.
- **Financial rows are append-only** (`docs/19`). Never `UPDATE` or `DELETE` a ledger entry — corrections are new entries.
- **Migrations are reversible where practical**, backward-compatible during rolling deploys, tested, and safe for existing data.
- **Large-table changes use expand → migrate → backfill → verify → switch → contract.** Never a blocking destructive migration on a live table.
- **PostGIS for geospatial** — proximity, zones, geofences. Index geometry columns; a sequential scan over driver locations will not survive production.

# Core Tables (`docs/13`)

`users` · `drivers` · `vehicles` · `vehicle_capabilities` · `driver_vehicles` · `jobs` · `job_stops` · `assignments` · `driver_locations` · `payments` / `ledger_entries` · `merchants` / `merchant_orders` · `support_cases` / `audit_events`

Read `docs/13-database-schema.md` for the column lists before creating any of these — they already have a defined shape.

# Workflow

1. Confirm the table's owning module and that it does not already exist.
2. Write the invariants as sentences; turn each into a constraint where the database can hold it.
3. Write the migration and its reverse.
4. Apply, verify constraints actually reject bad rows, roll back, re-apply.
5. Add indexes for the queries you are actually writing — measure rather than guess.

# Verification

Level 3 for schema change: migration up, constraints proven by attempting a violation, migration down, up again. Level 5 for anything touching `ledger_entries`, `payments`, or `assignments` — include a concurrency test.

A migration that has never been rolled back once is unverified.

# Blocking Conditions

- The migration is destructive to existing data → stop; explicit approval required.
- A required invariant cannot be expressed as a constraint and has no safe application-level equivalent → record it before proceeding.

# Relevant Documentation

`docs/13-database-schema.md` · `docs/192-database-architecture.md` · `docs/413-postgres-schema-specification.md` · `docs/414-postgres-index-specification.md` · `docs/415-postgis-schema.md` · `docs/417-redis-keyspace-specification.md` · `docs/418-redis-distributed-locks.md` · `docs/379-transaction-boundaries.md`
