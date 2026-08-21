# Technical Blueprint

## Locked Stack

- Customer/Driver: React Native + Expo + TypeScript
- Merchant/Admin: React + Vite + TypeScript
- Marketing: Next.js
- Backend: Go
- API: REST + WebSocket
- Database: PostgreSQL + PostGIS
- Cache: Redis
- Messaging: NATS
- Storage: S3-compatible
- Infrastructure: Docker + AWS ECS/Fargate + Terraform
- CI/CD: GitHub Actions
- Observability: Sentry + OpenTelemetry
- Analytics: PostHog

## Architecture

```text
Mobile / Dashboards
       |
 REST + WebSocket
       |
     Go API
   /   |    \
Postgres Redis NATS
 +PostGIS
```

Start as a modular monolith. Extract services only when scale or ownership requires it.
