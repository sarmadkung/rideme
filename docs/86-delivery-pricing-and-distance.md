# 86 — Delivery Pricing & Distance

## Objective
Calculate parcel/cargo delivery prices consistently while allowing service-specific pricing.

## Pricing Inputs
Possible inputs:
- base fee
- distance
- duration
- vehicle type
- cargo weight
- cargo volume
- number of stops
- waiting time
- tolls
- peak multiplier
- scheduled surcharge
- service zone

## Quote
```text
base
+ distance
+ service modifiers
+ cargo modifiers
+ waiting/tolls where applicable
+ tax
- discounts
= total
```

## Distance Source
Use routing distance/time when accuracy matters.

Straight-line distance is suitable for early candidate filtering but not final customer pricing when road routing is available.

## Versioning
Store:
```text
pricing_version
quote_id
pricing_inputs
```

Historical orders must not change when pricing configuration changes.

## Definition of Done
Every delivery quote is reproducible from its stored pricing version and inputs.
