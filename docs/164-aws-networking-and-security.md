# 164 — AWS Networking & Security

## Objective
Create a secure network foundation.

## Typical Layout
```text
VPC
 ├── Public Subnets
 │    └── Load Balancer
 └── Private Subnets
      ├── ECS Services
      ├── Workers
      └── Data Services
```

## Controls
- security groups
- IAM roles
- private database access
- controlled outbound traffic
- TLS
- secrets management
- audit logging

## Principle
Databases and internal services should not be directly exposed to the public internet.

## Definition of Done
Only intended entry points are publicly accessible and internal resources use private networking.
