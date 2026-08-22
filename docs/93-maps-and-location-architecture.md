# 93 — Maps & Location Architecture

## Objective
Create a provider-agnostic geographic layer for rides, grocery, parcel and cargo services.

## Core Capabilities
- geocoding
- reverse geocoding
- routing
- distance/time estimation
- map display
- place search
- geofencing
- driver location
- ETA

## Architecture
```text
React Native / React
        ↓
Location SDK Layer
        ↓
Backend Location Service
        ↓
Provider Adapter
        ↓
Map / Routing Provider
```

Do not spread provider-specific APIs throughout the application.

## Provider Abstraction
Create interfaces such as:
```text
Geocoder
Router
PlaceSearcher
MapProvider
```

## Provider Strategy
The system should be able to switch or combine providers by use case and geography.

## Definition of Done
Application code depends on internal geographic interfaces rather than directly on one map vendor.
