# 165 — Docker & Container Strategy

## Objective
Standardize packaging and deployment of backend services.

## Container Principles
Each deployable service should have:
- reproducible Dockerfile
- pinned dependencies where appropriate
- non-root runtime where possible
- health endpoint
- graceful shutdown
- structured logs

## Images
```text
Source
 ↓
Build
 ↓
Test
 ↓
Container Image
 ↓
Registry
 ↓
Deployment
```

## Image Optimization
Use:
- multi-stage builds
- minimal runtime image
- dependency caching

## Definition of Done
Every production service can be built and run consistently through its container image.
