# Dispatch, Pricing & Optimization Specification

## Dispatch Goal

Assign the best compatible vehicle/driver, not simply the nearest driver.

## Candidate Filtering

Reject candidate if:
- Vehicle capability mismatch
- Capacity insufficient
- Driver offline
- Driver unavailable
- Required documents expired
- Vehicle not verified
- Restricted zone
- Safety/risk rule triggered

## Candidate Scoring

Example conceptual score:

```text
score =
  w1 * eta_score
+ w2 * vehicle_fit
+ w3 * driver_reliability
+ w4 * route_compatibility
+ w5 * price_fit
+ w6 * customer_preference
+ w7 * destination_demand
- w8 * empty_km
- w9 * cancellation_risk
```

Weights should be configurable and learned from real outcomes later.

## Route Compatibility

A driver traveling:

Lahore → Gujranwala

should receive priority for jobs that follow the same route.

This reduces empty kilometers.

## Return Load

Before a long-distance job completes, search for compatible jobs from the destination toward the driver's likely return route.

Display:

- Cargo
- Route
- Weight
- Price
- Pickup time
- Compatibility score

## Pricing

### Instant Ride

```text
base
+ distance
+ time
+ demand
+ vehicle adjustment
```

### Parcel

```text
base
+ distance
+ size/weight
+ urgency
```

### Cargo

```text
base
+ distance
+ vehicle
+ capacity
+ loading
+ waiting
+ schedule
```

## Price Range

Where uncertainty is material, show a range rather than false precision.

Example:

```text
Estimated fare:
Rs 1,800–2,100
```

## Driver Earnings Preview

Before acceptance:

```text
Gross: Rs 650
Distance: 11.2 km
Estimated fuel: Rs 125
Platform fee: Rs 55
Estimated net: Rs 470
```

## Cancellation Rules

Customer:
- Before driver movement: no/low fee
- After meaningful driver travel: compensation
- After arrival: higher cancellation fee

Driver:
- Repeated avoidable cancellation lowers reliability
- Legitimate operational/safety reasons are classified separately

## Long-Term Optimization

Later introduce:
- demand forecasting
- zone repositioning
- batch delivery
- route optimization
- return-load prediction
- driver shift planning
