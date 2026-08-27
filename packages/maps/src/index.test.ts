import { describe, expect, it } from 'vitest';
import { haversineMeters, isLatLng } from './index.js';

const lahoreCentre = { lat: 31.5204, lng: 74.3587 };
const lahoreAirport = { lat: 31.5216, lng: 74.4036 };

describe('haversineMeters', () => {
  it('is zero for identical points', () => {
    expect(haversineMeters(lahoreCentre, lahoreCentre)).toBe(0);
  });

  it('matches a known distance within 1%', () => {
    const expected = 4_250;
    const actual = haversineMeters(lahoreCentre, lahoreAirport);
    expect(Math.abs(actual - expected) / expected).toBeLessThan(0.01);
  });

  it('is symmetric', () => {
    expect(haversineMeters(lahoreCentre, lahoreAirport)).toBeCloseTo(
      haversineMeters(lahoreAirport, lahoreCentre),
      6,
    );
  });
});

describe('isLatLng', () => {
  it('rejects out-of-range and malformed input', () => {
    expect(isLatLng({ lat: 31.5, lng: 74.3 })).toBe(true);
    expect(isLatLng({ lat: 91, lng: 0 })).toBe(false);
    expect(isLatLng({ lat: '31.5', lng: 74.3 })).toBe(false);
    expect(isLatLng(null)).toBe(false);
  });
});
