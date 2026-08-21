# React Native & Native Module Architecture

React Native owns screens, navigation, UI and application flows. Swift/Kotlin owns platform-specific capabilities such as background location, sensors, biometrics, secure storage and specialized navigation.

## Shared Interface

```ts
interface LocationService {
  startTracking(options: TrackingOptions): Promise<void>;
  stopTracking(): Promise<void>;
  getCurrentLocation(): Promise<Location>;
}
```

Implementation:

```text
TypeScript interface
       |
   Native module
    /       \
 Swift     Kotlin
```

Do not duplicate pricing, dispatch or job rules in native code.

## Location Pipeline

```text
GPS -> Native filtering -> batching -> React Native -> WebSocket -> Go -> Redis/NATS
```

Use Expo development builds for custom native modules. Keep custom native code limited to capabilities that need it.
