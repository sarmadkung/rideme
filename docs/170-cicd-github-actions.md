# 170 — CI/CD with GitHub Actions

## Objective
Automate quality checks and production deployments.

## Pipeline
```text
Pull Request
 ↓
Lint
 ↓
Type Check
 ↓
Unit Tests
 ↓
Build
 ↓
E2E where applicable
 ↓
Review
```

## Deployment
```text
main
 ↓
Build Image
 ↓
Security Checks
 ↓
Push Registry
 ↓
Deploy Staging
 ↓
Smoke Tests
 ↓
Production
```

## Monorepo
Use affected-project detection where possible to avoid rebuilding unrelated applications.

## Definition of Done
Every production deployment is reproducible and traceable to a commit.
