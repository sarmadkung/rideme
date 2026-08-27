/**
 * Map abstractions and shared geographic utilities (document 23).
 *
 * Provider-specific code (Google, Mapbox, OSRM) is deliberately absent — the
 * provider adapter and its fallback strategy belong to the maps slice.
 */

export interface LatLng {
  readonly lat: number;
  readonly lng: number;
}

export const EARTH_RADIUS_METERS = 6_371_008.8;

export function isLatLng(value: unknown): value is LatLng {
  if (typeof value !== 'object' || value === null) return false;
  const candidate = value as Record<string, unknown>;
  return (
    typeof candidate['lat'] === 'number' &&
    typeof candidate['lng'] === 'number' &&
    Number.isFinite(candidate['lat']) &&
    Number.isFinite(candidate['lng']) &&
    Math.abs(candidate['lat'] as number) <= 90 &&
    Math.abs(candidate['lng'] as number) <= 180
  );
}

const toRadians = (degrees: number): number => (degrees * Math.PI) / 180;

/**
 * Great-circle distance in metres. Straight-line only — road distance comes
 * from a routing provider and must never be approximated for pricing.
 */
export function haversineMeters(from: LatLng, to: LatLng): number {
  const dLat = toRadians(to.lat - from.lat);
  const dLng = toRadians(to.lng - from.lng);
  const a =
    Math.sin(dLat / 2) ** 2 +
    Math.cos(toRadians(from.lat)) * Math.cos(toRadians(to.lat)) * Math.sin(dLng / 2) ** 2;
  return 2 * EARTH_RADIUS_METERS * Math.asin(Math.min(1, Math.sqrt(a)));
}
