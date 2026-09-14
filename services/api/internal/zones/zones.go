// Package zones is the geographic boundary pricing and dispatch key off
// (document 97). It exists so a business rule can be tied to "inside this
// circle" rather than to a city-name string, which is exactly what ADR-004
// names this module for.
//
// v1 keeps geometry to a circle — center point plus radius — reusing the
// ST_DWithin pattern already proven in booking.Store.Supply, rather than
// introducing polygon storage this schema has never needed. The fuller
// scope from document 143 (availability/hours/surge rules, deterministic
// overlap priority beyond "smallest radius wins", true polygon boundaries)
// is deferred.
package zones

import (
	"context"
	"time"

	"github.com/sarmadkung/rideme/services/api/pkg/httpx"
)

// Status is whether a zone is currently used by lookups.
type Status string

const (
	StatusActive   Status = "ACTIVE"
	StatusInactive Status = "INACTIVE"
)

// Zone is a circular service area.
type Zone struct {
	ID           string
	Name         string
	City         string
	Lat          float64
	Lon          float64
	RadiusMeters int
	Status       Status
	CreatedAt    time.Time
}

// Validate checks the fields an admin controls when creating a zone.
//
// Status and CreatedAt are not validated here: the store sets both.
func (z Zone) Validate() error {
	details := map[string]string{}
	if z.Name == "" {
		details["name"] = "required"
	}
	if z.Lat < -90 || z.Lat > 90 {
		details["latitude"] = "must be between -90 and 90"
	}
	if z.Lon < -180 || z.Lon > 180 {
		details["longitude"] = "must be between -180 and 180"
	}
	if z.RadiusMeters <= 0 || z.RadiusMeters > 200000 {
		details["radius_meters"] = "must be between 1 and 200000"
	}
	if len(details) > 0 {
		return httpx.Validation("a zone has an invalid field", details)
	}
	return nil
}

// Service validates before the store persists. It is thin by design — a
// circle has little business logic beyond "is this a sane circle" — and grows
// once document 143's rules layer (availability, surge, hours) lands.
type Service struct{ store *Store }

func NewService(store *Store) *Service { return &Service{store: store} }

// Create validates and persists a new zone.
func (s *Service) Create(ctx context.Context, z Zone) (Zone, error) {
	if err := z.Validate(); err != nil {
		return Zone{}, err
	}
	return s.store.Create(ctx, z)
}

// List returns every zone, most recently created first.
func (s *Service) List(ctx context.Context) ([]Zone, error) {
	return s.store.List(ctx)
}
