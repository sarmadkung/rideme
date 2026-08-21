# Architecture & Product Decision Log

## ADR-001: Job-Centric Domain

Decision:
Represent rides, parcels, grocery deliveries and cargo as specialized Jobs.

Reason:
Allows one dispatch, tracking, payment and support system.

## ADR-002: Vehicle Capability Model

Decision:
Vehicles have capabilities rather than belonging to one service.

Reason:
Enables multi-service driver economics.

## ADR-003: Modular Monolith First

Decision:
Start with a modular monolith.

Reason:
Faster iteration and lower operational complexity.

## ADR-004: PostgreSQL + PostGIS

Decision:
Use PostgreSQL/PostGIS as the core operational database.

Reason:
Strong transactional model plus geospatial querying.

## ADR-005: Grocery Is Not Initial Core

Decision:
Do not own grocery inventory initially.

Reason:
High operational complexity and existing strong competitors.

## ADR-006: B2B Logistics Is Strategic

Decision:
Treat merchant logistics as a first-class product.

Reason:
More predictable repeat demand and stronger retention potential.

## ADR-007: Driver Profitability Is a Product Metric

Decision:
Show estimated net earnings and optimize jobs for driver profitability.

Reason:
Supply retention is a core marketplace constraint.

## ADR-008: Return Loads Are Strategic

Decision:
Build return-load matching after the initial marketplace is stable.

Reason:
Reduces deadhead kilometers and improves vehicle economics.

## ADR-009: One City First

Decision:
Launch one city and a small dense zone.

Reason:
Marketplace liquidity is more important than geographic coverage.

## ADR-010: Avoid Nationwide Subsidies

Decision:
Do not depend on indefinite below-cost pricing.

Reason:
Ride-hailing economics are highly competitive and difficult to sustain.
