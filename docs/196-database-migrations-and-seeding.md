# 196 — Database Migrations & Seeding

## Objective
Define repeatable schema evolution and development data.

## Migration rules
- One logical change per migration.
- Forward migrations must be deterministic.
- Destructive changes require explicit migration planning.
- Large data migrations must be separated from schema migrations where practical.

## Environments
Development, test, staging, and production must use the same migration mechanism.

## Seeds
Development seeds should provide:
- users
- drivers
- vehicles
- service types
- sample merchants
- sample products
- sample service zones

Test fixtures should be isolated from production seeds.

## Agent tasks
- Implement migration runner.
- Implement development seed command.
- Implement test fixture factory.
- Add CI migration checks.

## Acceptance criteria
A developer can create a fresh environment with one documented setup process.
