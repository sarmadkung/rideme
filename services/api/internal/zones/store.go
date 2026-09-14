package zones

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store persists zones and resolves a point to the zone that contains it.
type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// ErrNotFound reports that no zone contains the given point.
var ErrNotFound = errors.New("zones: no zone contains this point")

// Create persists a new active zone and returns it as stored (id, status and
// created_at are all server-generated).
func (s *Store) Create(ctx context.Context, z Zone) (Zone, error) {
	err := s.pool.QueryRow(ctx,
		`INSERT INTO service_zones (name, city, center, radius_meters, status)
		 VALUES ($1, $2, ST_SetSRID(ST_MakePoint($3, $4), 4326)::geography, $5, $6)
		 RETURNING id::text, status, created_at`,
		z.Name, nullableString(z.City), z.Lon, z.Lat, z.RadiusMeters, StatusActive).
		Scan(&z.ID, &z.Status, &z.CreatedAt)
	if err != nil {
		return Zone{}, fmt.Errorf("create zone: %w", err)
	}
	return z, nil
}

// List returns every zone, most recently created first.
func (s *Store) List(ctx context.Context) ([]Zone, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, name, city, ST_Y(center::geometry), ST_X(center::geometry),
		        radius_meters, status, created_at
		   FROM service_zones
		  ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list zones: %w", err)
	}
	defer rows.Close()

	var out []Zone
	for rows.Next() {
		var z Zone
		var city *string
		if err := rows.Scan(&z.ID, &z.Name, &city, &z.Lat, &z.Lon,
			&z.RadiusMeters, &z.Status, &z.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan zone: %w", err)
		}
		if city != nil {
			z.City = *city
		}
		out = append(out, z)
	}
	return out, rows.Err()
}

// FindContaining returns the active zone containing (lat, lon), preferring
// the smallest radius when zones overlap — the deterministic "most specific
// wins" rule for v1 (document 143's fuller priority system is deferred).
// Equal radii break the tie by newest-first, so the rule stays deterministic
// rather than depending on whatever order the database happens to return.
//
// A miss is not an error the caller must treat specially in most cases (a
// point outside every zone falls back to city/universal pricing), so callers
// typically check errors.Is(err, ErrNotFound) and proceed with an empty zone.
func (s *Store) FindContaining(ctx context.Context, lat, lon float64) (Zone, error) {
	var z Zone
	var city *string
	err := s.pool.QueryRow(ctx,
		`SELECT id::text, name, city, ST_Y(center::geometry), ST_X(center::geometry),
		        radius_meters, status, created_at
		   FROM service_zones
		  WHERE status = $3
		    AND ST_DWithin(center, ST_MakePoint($1, $2)::geography, radius_meters)
		  ORDER BY radius_meters ASC, created_at DESC
		  LIMIT 1`,
		lon, lat, StatusActive).
		Scan(&z.ID, &z.Name, &city, &z.Lat, &z.Lon, &z.RadiusMeters, &z.Status, &z.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Zone{}, ErrNotFound
	}
	if err != nil {
		return Zone{}, fmt.Errorf("find containing zone: %w", err)
	}
	if city != nil {
		z.City = *city
	}
	return z, nil
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
