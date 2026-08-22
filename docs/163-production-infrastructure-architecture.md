# 163 — Production Infrastructure Architecture

## Objective
Define a production-ready AWS architecture for the logistics platform.

## High-Level
```text
Users
 ↓
CDN / Load Balancer
 ↓
API / Realtime Services
 ↓
Domain Services
 ├── PostgreSQL/PostGIS
 ├── Redis
 ├── Object Storage
 └── Event/Queue Infrastructure
```

## AWS Principles
- managed services where practical
- private networking for databases
- least-privilege IAM
- infrastructure as code
- multi-AZ for critical services
- observable and recoverable production

## Application
```text
React Native
React Admin
Backend APIs
Realtime Gateway
Workers
Scheduled Jobs
```

## Definition of Done
Production infrastructure has documented network, compute, data and operational boundaries.
