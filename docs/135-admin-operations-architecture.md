# 135 — Admin & Operations Architecture

## Objective
Provide a secure operational control plane for the logistics platform.

## Domains
- users
- drivers
- vehicles
- merchants
- orders/jobs
- dispatch
- pricing
- zones
- payouts
- incidents
- support
- configuration
- audit

## Principle
Admin actions must use domain APIs and workflows rather than direct database manipulation.

## Architecture
```text
React Admin Dashboard
        ↓
Admin API
        ↓
Domain Services
        ↓
Operational Data
```

## Access
Every admin capability is permission-controlled and audited.

## Definition of Done
Operations can manage the platform without direct production database access.
