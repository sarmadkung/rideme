# 104 — Map Provider Cost & Fallback Strategy

## Objective
Control map-provider cost while avoiding operational dependency on one vendor.

## Cost Drivers
Monitor:
- geocoding calls
- reverse geocoding
- route requests
- matrix requests
- map tile usage
- navigation sessions

## Optimization
- cache stable geocoding
- debounce search
- avoid duplicate route requests
- reuse route estimates
- batch matrix queries
- throttle live tracking
- calculate only when state changes materially

## Provider Fallback
Possible architecture:
```text
Primary Provider
      ↓ failure
Fallback Provider
```

Fallback should be configured per operation because not all providers offer identical capabilities.

## Observability
Track:
```text
requests
success rate
latency
cost
fallback rate
```

## Definition of Done
Map usage has measurable cost and reliability controls before production scale.
