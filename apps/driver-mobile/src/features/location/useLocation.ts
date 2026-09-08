import { useCallback, useEffect, useRef, useState } from 'react';
import * as Location from 'expo-location';
import type { PositionInput } from '@platform/api-client';

/**
 * Where the driver actually is.
 *
 * This replaces the fixed coordinate the shell reported before. A driver going
 * online from the wrong point is offered jobs across the city, which is worse
 * than a driver who cannot go online at all — so every state that is not "we
 * have a real fix" is surfaced as such rather than substituted for.
 *
 * Foreground only. Background tracking needs a config plugin, a foreground
 * service on Android, and measurement on a real device; it is its own slice.
 */

export type LocationStage =
  /** Permission has not been answered yet, or the first fix has not arrived. */
  | 'starting'
  /** The driver refused. Recoverable — they can grant it later. */
  | 'denied'
  /** Location services are switched off device-wide. */
  | 'disabled'
  /** The platform failed in a way the app cannot classify. */
  | 'unavailable'
  /** A real fix is in hand. */
  | 'tracking';

export interface LocationState {
  /** The most recent acceptable fix, or null when there is not one yet. */
  position: PositionInput | null;
  stage: LocationStage;
  /** Why there is no position, in words a driver can act on. */
  message: string | null;
}

export interface LocationActions {
  /** Ask again after the driver has changed the setting. */
  retry(): void;
}

export interface LocationOptions {
  /**
   * How far the driver must move before a new fix is delivered, and how long
   * between fixes at most.
   *
   * Engineering defaults, not a product decision. The real values depend on
   * what dispatch needs against what the battery can afford, and that trade-off
   * is recorded as BD-20 in BLOCKED_TASKS.md — it wants measurement on a real
   * low-end device, which BD-19 also blocks on.
   */
  distanceIntervalM?: number;
  timeIntervalMs?: number;
}

const DEFAULT_DISTANCE_INTERVAL_M = 25;
const DEFAULT_TIME_INTERVAL_MS = 5000;

/**
 * The worst fix still worth reporting.
 *
 * A 100 m fix is fine for dispatch, which matches drivers to a pickup rather
 * than to a doorstep. Beyond that the fix is worse than the one before it, so
 * the previous one is kept — but only once there *is* a previous one: refusing
 * every rough fix would strand a driver who starts their shift indoors.
 */
const ACCEPTABLE_ACCURACY_M = 100;

const MESSAGES: Record<Exclude<LocationStage, 'tracking'>, string> = {
  starting: 'Waiting for your location.',
  denied: 'RideMe needs your location to send you nearby jobs. Enable it in Settings.',
  disabled: 'Location services are off. Turn them on to go online.',
  unavailable: 'Your location is unavailable right now.',
};

/**
 * Maps one platform fix onto the wire shape.
 *
 * Heading and speed are absent on a stationary device and are reported as -1 by
 * some Android hardware. An omitted field says "unknown"; -1 would say the
 * driver is facing a direction that does not exist.
 */
export function toPositionInput(fix: Location.LocationObject): PositionInput {
  const { latitude, longitude, accuracy, heading, speed } = fix.coords;
  return {
    latitude,
    longitude,
    // Document 048 times a fix by when it was recorded, not when it arrived.
    recordedAt: new Date(fix.timestamp),
    ...(typeof accuracy === 'number' && accuracy >= 0 ? { accuracyM: accuracy } : {}),
    ...(typeof heading === 'number' && heading >= 0 ? { headingDeg: heading } : {}),
    ...(typeof speed === 'number' && speed >= 0 ? { speedMps: speed } : {}),
  };
}

export function useLocation(options: LocationOptions = {}): LocationState & LocationActions {
  const distanceInterval = options.distanceIntervalM ?? DEFAULT_DISTANCE_INTERVAL_M;
  const timeInterval = options.timeIntervalMs ?? DEFAULT_TIME_INTERVAL_MS;

  const [position, setPosition] = useState<PositionInput | null>(null);
  const [stage, setStage] = useState<LocationStage>('starting');
  const [attempt, setAttempt] = useState(0);

  // Read inside the subscription callback without making it a dependency:
  // re-subscribing on every fix would restart the GPS session continuously.
  const latest = useRef<PositionInput | null>(null);
  latest.current = position;

  const retry = useCallback(() => {
    setStage('starting');
    setAttempt((n) => n + 1);
  }, []);

  useEffect(() => {
    let cancelled = false;
    let subscription: Location.LocationSubscription | null = null;

    void (async () => {
      try {
        const services = await Location.hasServicesEnabledAsync();
        if (cancelled) return;
        if (!services) {
          setStage('disabled');
          return;
        }

        const { granted } = await Location.requestForegroundPermissionsAsync();
        if (cancelled) return;
        if (!granted) {
          setStage('denied');
          return;
        }

        subscription = await Location.watchPositionAsync(
          { accuracy: Location.Accuracy.High, distanceInterval, timeInterval },
          (fix) => {
            const next = toPositionInput(fix);
            // Keep the better of the two rather than moving the driver to a
            // point the device is not confident about.
            if (
              latest.current !== null &&
              next.accuracyM !== undefined &&
              next.accuracyM > ACCEPTABLE_ACCURACY_M
            ) {
              return;
            }
            setPosition(next);
            setStage('tracking');
          },
        );

        // The permission dialog and the first fix can both resolve after the
        // component is gone; a subscription created then would never be torn
        // down by the cleanup that already ran.
        if (cancelled) {
          subscription.remove();
          subscription = null;
        }
      } catch {
        if (!cancelled) setStage('unavailable');
      }
    })();

    return () => {
      cancelled = true;
      subscription?.remove();
    };
  }, [distanceInterval, timeInterval, attempt]);

  return {
    position,
    stage,
    message: stage === 'tracking' ? null : MESSAGES[stage],
    retry,
  };
}
