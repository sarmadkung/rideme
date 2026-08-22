# 99 — React Native Native Location Strategy

## Objective
Use React Native for shared product logic while using native modules only where platform capabilities require them.

## Shared Layer
Keep shared:
- screens
- domain logic
- state
- API contracts
- validation
- location event model

## Native Layer
Use platform-specific modules for:
- background location
- foreground services on Android
- iOS background location behavior
- battery optimization
- OS permissions
- native geofencing where required

## Architecture
```text
Shared Location Service
        ↓
Platform Adapter
   ┌────┴────┐
 Android   iOS
```

The application does **not** need separate screens or separate business logic merely because native modules are used.

## Native APIs
Expose a small stable interface:
```text
startTracking()
stopTracking()
requestPermission()
getCurrentLocation()
subscribe()
```

## Definition of Done
Platform-specific code remains isolated and the majority of location logic stays shared.
