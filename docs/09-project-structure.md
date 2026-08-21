# Recommended Project Structure

## Repository

```text
logistics-platform/
├── apps/
│   ├── customer-mobile/
│   ├── driver-mobile/
│   ├── merchant-web/
│   ├── admin-web/
│   └── marketing-web/
│
├── services/
│   └── api/
│
├── packages/
│   ├── ui/
│   ├── types/
│   ├── validation/
│   ├── maps/
│   ├── auth/
│   └── config/
│
├── infra/
│   ├── docker/
│   ├── terraform/
│   └── environments/
│
├── docs/
│   ├── 00-project-vision.md
│   ├── 01-competitive-analysis.md
│   ├── 02-business-model.md
│   ├── 03-mvp-prd.md
│   ├── 04-domain-architecture.md
│   ├── 05-dispatch-pricing.md
│   ├── 06-business-go-to-market.md
│   ├── 07-metrics-operations.md
│   └── 08-roadmap.md
│
└── README.md
```

## Backend Modules

```text
api/
├── identity/
├── users/
├── drivers/
├── vehicles/
├── documents/
├── jobs/
├── dispatch/
├── pricing/
├── payments/
├── wallet/
├── ratings/
├── support/
├── merchants/
├── notifications/
├── zones/
├── fraud/
└── analytics/
```

## Important Rule

Keep domain logic independent from HTTP handlers, database adapters and external providers.

Use interfaces around:
- Maps
- Payments
- Notifications
- Storage
- Identity verification

This allows providers to change without rewriting business logic.
