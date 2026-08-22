# 171 — Secrets, Configuration & Environments

## Objective
Separate configuration from application code and protect secrets.

## Environments
```text
local
development
staging
production
```

## Configuration
Examples:
- API endpoints
- feature configuration
- provider IDs
- service settings

## Secrets
Examples:
- database credentials
- API keys
- signing keys
- payment secrets

Never commit secrets to Git.

## Rotation
Secrets should support controlled rotation without requiring source-code changes.

## Definition of Done
Secrets are managed by an appropriate secret store and environments are isolated.
