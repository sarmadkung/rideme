# 157 — Data Warehouse & Analytics Storage

## Objective
Separate analytical workloads from transactional systems.

## Logical Layers
```text
Raw Events
   ↓
Staging
   ↓
Cleaned Models
   ↓
Aggregates
   ↓
BI
```

## Data Sources
- PostgreSQL
- MongoDB where used
- event streams
- payment/ledger records
- application events

## Data Quality
Validate:
- duplicates
- missing keys
- invalid timestamps
- schema drift
- unexpected values

## Architecture Principle
Start simple. A warehouse can be introduced when operational analytics volume and requirements justify it.

## Definition of Done
Analytics storage can scale independently from production transactional databases.
