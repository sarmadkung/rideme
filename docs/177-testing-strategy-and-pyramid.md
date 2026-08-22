# 177 — Testing Strategy & Pyramid

## Objective
Define a testing strategy covering mobile, web, backend, realtime and operational workflows.

## Pyramid
```text
        E2E
      /     \
 Integration
    /       \
    Unit / Component
```

## Tests
- unit
- component
- integration
- API
- contract
- realtime
- E2E
- load/performance
- security

## Principle
Most business logic should be validated with fast unit/integration tests. E2E tests cover critical user journeys rather than every implementation detail.

## Critical Journeys
- registration
- driver onboarding
- ride booking
- grocery order
- parcel delivery
- cargo booking
- dispatch
- payment
- cancellation
- support
- SOS

## Definition of Done
Every critical domain has automated tests at appropriate layers.
