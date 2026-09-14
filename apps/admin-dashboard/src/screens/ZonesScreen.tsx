import { useState } from 'react';
import { tokens } from '@platform/ui';
import type { ApiClient } from '@platform/api-client';
import { useZones } from '../features/zones/useZones';

/**
 * Service zones (document 97): the circular boundaries pricing keys off. v1
 * keeps geometry to a center point and a radius — no map, just three numbers
 * — which is enough to prove zone-scoped pricing without the much larger
 * effort a real polygon-drawing tool would take.
 */
export function ZonesScreen({ client }: { client: ApiClient }) {
  const zones = useZones(client);
  const [name, setName] = useState('');
  const [city, setCity] = useState('');
  const [lat, setLat] = useState('');
  const [lon, setLon] = useState('');
  const [radiusKm, setRadiusKm] = useState('');

  const onCreate = async () => {
    const ok = await zones.create({
      name,
      ...(city ? { city } : {}),
      latitude: Number(lat),
      longitude: Number(lon),
      radiusMeters: Math.round(Number(radiusKm) * 1000),
    });
    if (ok) {
      setName('');
      setCity('');
      setLat('');
      setLon('');
      setRadiusKm('');
    }
  };

  return (
    <div>
      <h2 style={styles.heading}>Service zones</h2>
      <p style={styles.hint}>
        A circle: center point plus radius. Pricing scoped to a zone outranks pricing scoped to a
        city, which outranks pricing with neither.
      </p>

      <div style={styles.form}>
        <input
          style={styles.input}
          placeholder="Name (e.g. Liberty Market)"
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
        <input
          style={styles.input}
          placeholder="City (optional, for display)"
          value={city}
          onChange={(e) => setCity(e.target.value)}
        />
        <input
          style={styles.input}
          placeholder="Latitude"
          inputMode="decimal"
          value={lat}
          onChange={(e) => setLat(e.target.value)}
        />
        <input
          style={styles.input}
          placeholder="Longitude"
          inputMode="decimal"
          value={lon}
          onChange={(e) => setLon(e.target.value)}
        />
        <input
          style={styles.input}
          placeholder="Radius (km)"
          inputMode="decimal"
          value={radiusKm}
          onChange={(e) => setRadiusKm(e.target.value)}
        />
        <button
          style={styles.button}
          disabled={zones.creating || !name || !lat || !lon || !radiusKm}
          onClick={() => void onCreate()}
        >
          {zones.creating ? 'Creating…' : 'Create zone'}
        </button>
        {zones.createError && <p style={styles.error}>{zones.createError}</p>}
      </div>

      {zones.loading && zones.zones.length === 0 ? (
        <p style={styles.hint}>Loading…</p>
      ) : zones.error ? (
        <p style={styles.error}>{zones.error}</p>
      ) : zones.zones.length === 0 ? (
        <p style={styles.hint}>No zones yet.</p>
      ) : (
        <table style={styles.table}>
          <thead>
            <tr>
              {['Name', 'City', 'Center', 'Radius', 'Status'].map((h) => (
                <th key={h} style={styles.th}>
                  {h}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {zones.zones.map((z) => (
              <tr key={z.id}>
                <td style={styles.td}>{z.name}</td>
                <td style={styles.td}>{z.city ?? '—'}</td>
                <td style={styles.td}>
                  {z.latitude.toFixed(4)}, {z.longitude.toFixed(4)}
                </td>
                <td style={styles.td}>{(z.radius_meters / 1000).toFixed(1)} km</td>
                <td style={styles.td}>{z.status}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}

const styles: Record<string, React.CSSProperties> = {
  heading: { color: tokens.color.text, fontSize: tokens.fontSize.lg, margin: 0 },
  hint: { color: tokens.color.textMuted, fontSize: tokens.fontSize.sm },
  error: { color: tokens.color.danger, fontSize: tokens.fontSize.sm },
  form: {
    display: 'flex',
    flexWrap: 'wrap',
    gap: tokens.space.sm,
    alignItems: 'center',
    margin: `${tokens.space.md}px 0 ${tokens.space.lg}px`,
  },
  input: {
    background: tokens.color.surface,
    border: `1px solid ${tokens.color.border}`,
    borderRadius: tokens.radius.sm,
    color: tokens.color.text,
    fontSize: tokens.fontSize.sm,
    padding: tokens.space.sm,
    width: 160,
  },
  button: {
    background: tokens.color.accent,
    color: tokens.color.onAccent,
    border: 'none',
    borderRadius: tokens.radius.sm,
    padding: `${tokens.space.sm}px ${tokens.space.md}px`,
    fontSize: tokens.fontSize.sm,
    fontWeight: 600,
    cursor: 'pointer',
  },
  table: { width: '100%', borderCollapse: 'collapse' },
  th: {
    textAlign: 'left',
    color: tokens.color.textMuted,
    fontSize: tokens.fontSize.sm,
    borderBottom: `1px solid ${tokens.color.border}`,
    padding: tokens.space.sm,
  },
  td: {
    color: tokens.color.text,
    fontSize: tokens.fontSize.sm,
    borderBottom: `1px solid ${tokens.color.border}`,
    padding: tokens.space.sm,
  },
};
