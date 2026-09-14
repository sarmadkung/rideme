import { useState } from 'react';
import { tokens } from '@platform/ui';
import type { ApiClient } from '@platform/api-client';
import { useTariffs } from '../features/pricing/useTariffs';
import { useZones } from '../features/zones/useZones';

const JOB_TYPES = ['RIDE', 'PARCEL', 'GROCERY', 'CARGO', 'FREIGHT'];

/**
 * Pricing configuration (documents 34, 142). A tariff is never edited in
 * place: every submission here is a new row/version, never a change to an
 * existing one — matching how the server itself treats them.
 */
export function PricingScreen({ client }: { client: ApiClient }) {
  const tariffs = useTariffs(client);
  const zones = useZones(client);

  const [jobType, setJobType] = useState('RIDE');
  const [vehicleType, setVehicleType] = useState('');
  const [city, setCity] = useState('');
  const [zoneId, setZoneId] = useState('');
  const [version, setVersion] = useState('1');
  const [minimumFare, setMinimumFare] = useState('');
  const [base, setBase] = useState('');
  const [perKm, setPerKm] = useState('');
  const [perMinute, setPerMinute] = useState('');

  // PKR is entered as rupees here and converted to minor units (paisa) for
  // the wire — every amount on the server is an integer minor unit (ADR-008),
  // and asking an admin to type "10000" instead of "100" invites a fare that
  // is off by a factor of 100.
  const toMinor = (rupees: string) => Math.round(Number(rupees) * 100);

  const onCreate = async () => {
    const ok = await tariffs.create({
      jobType,
      ...(vehicleType ? { vehicleType } : {}),
      ...(city ? { city } : {}),
      ...(zoneId ? { zoneId } : {}),
      version: Number(version),
      minimumFareMinor: toMinor(minimumFare),
      baseMinor: toMinor(base),
      perKmMinor: toMinor(perKm),
      perMinuteMinor: toMinor(perMinute),
    });
    if (ok) {
      setVehicleType('');
      setCity('');
      setZoneId('');
      setMinimumFare('');
      setBase('');
      setPerKm('');
      setPerMinute('');
    }
  };

  return (
    <div>
      <h2 style={styles.heading}>Pricing</h2>
      <p style={styles.hint}>
        A zone-scoped tariff outranks a city-scoped one, which outranks one with neither. Amounts
        are PKR (converted to paisa on the wire).
      </p>

      <div style={styles.form}>
        <select style={styles.input} value={jobType} onChange={(e) => setJobType(e.target.value)}>
          {JOB_TYPES.map((t) => (
            <option key={t} value={t}>
              {t}
            </option>
          ))}
        </select>
        <input
          style={styles.input}
          placeholder="Vehicle type (optional)"
          value={vehicleType}
          onChange={(e) => setVehicleType(e.target.value)}
        />
        <input
          style={styles.input}
          placeholder="City (optional)"
          value={city}
          onChange={(e) => setCity(e.target.value)}
        />
        <select style={styles.input} value={zoneId} onChange={(e) => setZoneId(e.target.value)}>
          <option value="">No zone</option>
          {zones.zones.map((z) => (
            <option key={z.id} value={z.id}>
              {z.name}
            </option>
          ))}
        </select>
        <input
          style={styles.input}
          placeholder="Version"
          inputMode="numeric"
          value={version}
          onChange={(e) => setVersion(e.target.value)}
        />
        <input
          style={styles.input}
          placeholder="Minimum fare (PKR)"
          inputMode="decimal"
          value={minimumFare}
          onChange={(e) => setMinimumFare(e.target.value)}
        />
        <input
          style={styles.input}
          placeholder="Base fare (PKR)"
          inputMode="decimal"
          value={base}
          onChange={(e) => setBase(e.target.value)}
        />
        <input
          style={styles.input}
          placeholder="Per km (PKR)"
          inputMode="decimal"
          value={perKm}
          onChange={(e) => setPerKm(e.target.value)}
        />
        <input
          style={styles.input}
          placeholder="Per minute (PKR)"
          inputMode="decimal"
          value={perMinute}
          onChange={(e) => setPerMinute(e.target.value)}
        />
        <button
          style={styles.button}
          disabled={tariffs.creating || !minimumFare || !base}
          onClick={() => void onCreate()}
        >
          {tariffs.creating ? 'Creating…' : 'Create tariff'}
        </button>
        {tariffs.createError && <p style={styles.error}>{tariffs.createError}</p>}
      </div>

      {tariffs.loading && tariffs.tariffs.length === 0 ? (
        <p style={styles.hint}>Loading…</p>
      ) : tariffs.error ? (
        <p style={styles.error}>{tariffs.error}</p>
      ) : tariffs.tariffs.length === 0 ? (
        <p style={styles.hint}>No tariffs yet — nothing can be quoted until one exists.</p>
      ) : (
        <table style={styles.table}>
          <thead>
            <tr>
              {['Job', 'Vehicle', 'City', 'Zone', 'Ver', 'Base', 'Per km', 'Min fare'].map((h) => (
                <th key={h} style={styles.th}>
                  {h}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {tariffs.tariffs.map((t) => (
              <tr key={t.id}>
                <td style={styles.td}>{t.job_type}</td>
                <td style={styles.td}>{t.vehicle_type ?? '—'}</td>
                <td style={styles.td}>{t.city ?? '—'}</td>
                <td style={styles.td}>{zones.zones.find((z) => z.id === t.zone_id)?.name ?? '—'}</td>
                <td style={styles.td}>{t.version}</td>
                <td style={styles.td}>{(t.base_minor / 100).toFixed(2)}</td>
                <td style={styles.td}>{(t.per_km_minor / 100).toFixed(2)}</td>
                <td style={styles.td}>{(t.minimum_fare_minor / 100).toFixed(2)}</td>
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
