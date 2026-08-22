# 158 — Analytics Data Model

## Fact Tables
Examples:
```text
fact_rides
fact_deliveries
fact_orders
fact_payments
fact_support_cases
fact_driver_sessions
```

## Dimensions
```text
dim_customer
dim_driver
dim_vehicle
dim_merchant
dim_service
dim_zone
dim_date
dim_city
```

## Slowly Changing Data
Where historical analysis requires it, preserve relevant dimension history rather than replacing old values.

## Grain
Every fact table must explicitly define its grain.

Example:
```text
fact_delivery = one completed/attempted delivery job
```

## Definition of Done
Analytics models have documented grain and consistent relationships.
