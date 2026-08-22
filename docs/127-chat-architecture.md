# 127 — Chat Architecture

## Objective
Provide contextual communication between customers, drivers, merchants and support.

## Conversation Types
```text
CUSTOMER_DRIVER
CUSTOMER_MERCHANT
DRIVER_MERCHANT
CUSTOMER_SUPPORT
DRIVER_SUPPORT
MERCHANT_SUPPORT
```

## Context
Every operational conversation should reference:
```text
trip_id
order_id
job_id
support_case_id
```
where applicable.

## Architecture
```text
React Native / React
        ↓
Realtime Chat Gateway
        ↓
Chat Service
        ↓
Message Store
```

## Principle
Chat identity and permissions come from the platform, not arbitrary client-supplied participant IDs.

## Definition of Done
Participants can communicate only within conversations they are authorized to access.
