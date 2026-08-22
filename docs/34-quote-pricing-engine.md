# 34 — Quote & Pricing Engine

## Objective
Produce a transparent, deterministic quote before a customer confirms a job.

## Flow
```text
Requirements -> Validation -> Route Estimate -> Vehicle Eligibility
             -> Pricing -> Quote -> Customer Confirmation
```

## Quote
```text
Quote
├── id
├── job_type
├── vehicle_type
├── base_fare
├── distance_fare
├── time_fare
├── service_fee
├── waiting_estimate
├── demand_adjustment
├── discount
├── tax
├── total
├── currency
├── expires_at
└── pricing_version
```

## Formula
```text
total =
    base
  + distance_charge
  + time_charge
  + service_charge
  + loading_charge
  + waiting_charge
  + demand_adjustment
  + taxes
  - discounts
```

Rates are configuration, not hard-coded business logic.

## Pricing Configuration
Support configuration by city, zone, job type, vehicle type and time window, including minimum fare, per-km, per-minute, waiting, loading, service fee and bounded demand adjustment.

## Dynamic Pricing
Launch with bounded adjustments and operational visibility. Do not introduce uncontrolled surge.

## Quote Expiration
Quotes expire because supply, demand and route estimates can change.

## Price Lock
After confirmation, store a pricing snapshot and pricing version. Historical prices must not be recomputed from current configuration.

## Definition of Done
- quote endpoint works
- pricing is configuration-driven
- breakdown is complete
- expiration is enforced
- confirmed pricing is immutable
