# 149 — Analytics Architecture

## Objective
Create a scalable analytics layer for product, operations, finance and marketplace intelligence.

## Architecture
```text
Applications / Services
        ↓
Domain Events
        ↓
Event Collection
        ↓
Stream / Queue
        ↓
Analytics Storage
        ↓
Models / Aggregations
        ↓
Dashboards / Reports
```

## Principle
Operational databases remain optimized for transactions. Analytics workloads should not overload production systems.

## Domains
- customer
- driver
- merchant
- vehicle
- order
- delivery
- ride
- dispatch
- payment
- support
- safety

## Definition of Done
Business and operational analytics can be produced without running expensive analytical queries against primary transactional databases.
