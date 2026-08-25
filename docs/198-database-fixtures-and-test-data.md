# 198 — Database Fixtures & Test Data

## Objective
Provide deterministic data for automated tests and development.

## Fixture categories
- customer
- driver
- vehicle
- merchant
- store
- product
- service zone
- booking
- job
- payment
- incident

## Rules
- Fixtures must be deterministic.
- Avoid depending on production data.
- Generate realistic but non-sensitive data.
- Factories should allow overrides.

## Scenarios
Provide fixtures for:
- successful ride
- cancelled ride
- unassigned job
- completed delivery
- failed payment
- merchant order
- cargo booking
- driver offline
- network recovery

## Acceptance criteria
Tests can create isolated scenarios without manually inserting database rows.
