//go:build integration

package tests

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sarmadkung/rideme/services/api/internal/booking"
	"github.com/sarmadkung/rideme/services/api/internal/identity"
	"github.com/sarmadkung/rideme/services/api/internal/pricing"
	"github.com/sarmadkung/rideme/services/api/internal/zones"
	"github.com/sarmadkung/rideme/services/api/pkg/money"
)

func newZonesStore(t *testing.T) *zones.Store {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), env(t, "DATABASE_URL",
		"postgres://logistics:logistics@localhost:55432/logistics_dev?sslmode=disable"))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return zones.NewStore(pool)
}

// Two zones overlap at the same point; the smaller one is the more specific
// rule and must win (document 143's v1 overlap rule, in the absence of a
// full priority system).
func TestZoneFindContainingPrefersSmallerRadius(t *testing.T) {
	store := newZonesStore(t)
	ctx := context.Background()
	lat, lon := 31.5204, 74.3587

	big, err := store.Create(ctx, zones.Zone{Name: "Lahore-wide", Lat: lat, Lon: lon, RadiusMeters: 50000})
	if err != nil {
		t.Fatalf("create big zone: %v", err)
	}
	small, err := store.Create(ctx, zones.Zone{Name: "Liberty-small", Lat: lat, Lon: lon, RadiusMeters: 2000})
	if err != nil {
		t.Fatalf("create small zone: %v", err)
	}

	found, err := store.FindContaining(ctx, lat, lon)
	if err != nil {
		t.Fatalf("FindContaining: %v", err)
	}
	if found.ID != small.ID {
		t.Fatalf("resolved zone %q (%q), want the smaller zone %q (%q)",
			found.ID, found.Name, small.ID, small.Name)
	}
	_ = big
}

func TestZoneFindContainingMissesOutsideAnyZone(t *testing.T) {
	store := newZonesStore(t)
	ctx := context.Background()

	if _, err := store.Create(ctx, zones.Zone{
		Name: "Lahore", Lat: 31.5204, Lon: 74.3587, RadiusMeters: 2000,
	}); err != nil {
		t.Fatalf("create zone: %v", err)
	}

	// Karachi, roughly 1,100km from the zone above — well outside a 2km radius.
	_, err := store.FindContaining(ctx, 24.8607, 67.0011)
	if !errors.Is(err, zones.ErrNotFound) {
		t.Fatalf("FindContaining outside any zone: got %v, want ErrNotFound", err)
	}
}

// principalRouter wires a handler's routes behind a fake authenticator that
// injects the given principal directly, rather than requiring a real JWT —
// these tests are about RequireRole's behaviour, not about token issuance.
func principalRouter(routes func(mux *http.ServeMux, authenticate func(http.Handler) http.Handler), principal identity.Principal) http.Handler {
	mux := http.NewServeMux()
	routes(mux, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(identity.ContextWithPrincipal(r.Context(), principal)))
		})
	})
	return mux
}

func TestZonesAdminRoutesRejectNonAdmin(t *testing.T) {
	handler := zones.NewHandler(zones.NewService(newZonesStore(t)))
	router := principalRouter(handler.Routes, identity.Principal{
		UserID: "u1", Roles: []identity.Role{identity.RoleDriver},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/zones", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("GET /admin/zones as DRIVER: got %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestZonesAdminRoutesAllowAdmin(t *testing.T) {
	handler := zones.NewHandler(zones.NewService(newZonesStore(t)))
	router := principalRouter(handler.Routes, identity.Principal{
		UserID: "u1", Roles: []identity.Role{identity.RoleAdmin},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/zones", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /admin/zones as ADMIN: got %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestPricingAdminRoutesRejectNonAdmin(t *testing.T) {
	h := newBookingHarness(t)
	handler := booking.NewHandler(h.service, h.jobs, nil, h.store)
	router := principalRouter(handler.Routes, identity.Principal{
		UserID: "u1", Roles: []identity.Role{identity.RoleDriver},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/pricing/tariffs", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("GET /admin/pricing/tariffs as DRIVER: got %d, want %d", rec.Code, http.StatusForbidden)
	}
}

// Specificity: a zone-scoped tariff must outrank a city-scoped tariff, which
// must outrank a universal one (document 34: "by city, zone" — zone is the
// finer-grained dimension).
func TestTariffSpecificityZoneBeatsCityBeatsUniversal(t *testing.T) {
	h := newBookingHarness(t)
	ctx := context.Background()
	city := "LHR-" + time.Now().Format("150405.000000")

	zoneStore := newZonesStore(t)
	zone, err := zoneStore.Create(ctx, zones.Zone{
		Name: "test-zone", Lat: 31.5204, Lon: 74.3587, RadiusMeters: 2000,
	})
	if err != nil {
		t.Fatalf("create zone: %v", err)
	}

	// The universal row below has no city/zone to randomize, so the version
	// must vary across runs instead — otherwise a rerun collides with the
	// previous run's row on the (job_type, vehicle_type, city, zone_id,
	// version) unique constraint.
	version := int(time.Now().UnixNano() % 1_000_000)
	base := pricing.Tariff{
		JobType: "RIDE", VehicleType: "CAR", Version: version, Currency: money.PKR,
		BaseMinor: 1000, PerKMMinor: 100, MinimumFareMinor: 1000,
	}
	// The universal row matches ANY city for CAR/RIDE — including the random
	// throwaway cities other tests use to prove "no tariff configured"
	// refuses rather than guesses. It must not outlive this test, or it
	// silently turns every one of those tests into a false pass.
	deleteTariff := func(id string) {
		if _, err := h.pool.Exec(ctx, `DELETE FROM pricing_tariffs WHERE id = $1`, id); err != nil {
			t.Errorf("cleanup tariff %s: %v", id, err)
		}
	}

	universal := base
	universalID, err := h.store.SaveTariff(ctx, universal)
	if err != nil {
		t.Fatalf("save universal tariff: %v", err)
	}
	t.Cleanup(func() { deleteTariff(universalID) })

	cityScoped := base
	cityScoped.City = city
	cityScoped.BaseMinor = 2000
	cityID, err := h.store.SaveTariff(ctx, cityScoped)
	if err != nil {
		t.Fatalf("save city tariff: %v", err)
	}
	t.Cleanup(func() { deleteTariff(cityID) })

	zoneScoped := base
	zoneScoped.City = city
	zoneScoped.ZoneID = zone.ID
	zoneScoped.BaseMinor = 3000
	zoneTariffID, err := h.store.SaveTariff(ctx, zoneScoped)
	if err != nil {
		t.Fatalf("save zone tariff: %v", err)
	}
	t.Cleanup(func() { deleteTariff(zoneTariffID) })

	// A zone id is provided: the zone-scoped row must win over both fallbacks.
	got, err := h.store.Tariff(ctx, "RIDE", "CAR", city, zone.ID)
	if err != nil {
		t.Fatalf("Tariff with zone: %v", err)
	}
	if got.BaseMinor != zoneScoped.BaseMinor {
		t.Fatalf("BaseMinor = %d, want the zone-scoped tariff's %d", got.BaseMinor, zoneScoped.BaseMinor)
	}

	// No zone id: falls back to the city-scoped row over the universal one.
	got, err = h.store.Tariff(ctx, "RIDE", "CAR", city, "")
	if err != nil {
		t.Fatalf("Tariff without zone: %v", err)
	}
	if got.BaseMinor != cityScoped.BaseMinor {
		t.Fatalf("BaseMinor = %d, want the city-scoped tariff's %d", got.BaseMinor, cityScoped.BaseMinor)
	}

	// Neither zone nor city known: falls back to the universal row.
	got, err = h.store.Tariff(ctx, "RIDE", "CAR", "some-other-city", "")
	if err != nil {
		t.Fatalf("Tariff with neither: %v", err)
	}
	if got.BaseMinor != universal.BaseMinor {
		t.Fatalf("BaseMinor = %d, want the universal tariff's %d", got.BaseMinor, universal.BaseMinor)
	}
}
