# 94 — Geocoding & Address Model

## Objective
Convert customer-entered locations into reliable coordinates and normalized addresses.

## Address Flow
```text
Search
 ↓
Place Selection
 ↓
Coordinates
 ↓
Normalized Address
 ↓
Saved Address
```

## Address Data
```text
label
address_line
area
city
region
country
postal_code
latitude
longitude
place_id
```

## Important Rule
Coordinates are operationally important, but a coordinate alone is not a sufficient customer-facing address.

## Reverse Geocoding
Use for:
- driver display
- pickup confirmation
- operational tools

Do not overwrite customer-entered addresses blindly with provider-generated text.

## Precision
Track approximate vs precise location where useful.

## Saved Addresses
Customer may have:
- Home
- Work
- Other

## Definition of Done
Locations are normalized, validated and reusable across orders/jobs.
