package tracking_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sarmadkung/rideme/services/api/internal/identity"
	"github.com/sarmadkung/rideme/services/api/internal/tracking"
)

const (
	watcherID = "user-1"
	onTheWay  = "driver-7"
)

type stubPositions struct {
	session   tracking.Session
	live      bool
	sessErr   error
	current   tracking.Current
	found     bool
	authErr   error
	askedWith []ask
}

type ask struct {
	actorID, role, driverID, jobID string
	scope                          tracking.Scope
}

func (s *stubPositions) LiveSession(_ context.Context, _ string) (tracking.Session, bool, error) {
	if s.sessErr != nil {
		return tracking.Session{}, false, s.sessErr
	}
	return s.session, s.live, nil
}

func (s *stubPositions) AuthorizeView(_ context.Context, actorID, actorRole, driverID,
	jobID string, scope tracking.Scope) error {
	s.askedWith = append(s.askedWith, ask{actorID, actorRole, driverID, jobID, scope})
	return s.authErr
}

func (s *stubPositions) Current(_ context.Context, _ string) (tracking.Current, bool, error) {
	return s.current, s.found, nil
}

type stubOwnDriver struct{ id string }

func (s stubOwnDriver) DriverIDForUser(_ context.Context, _ string) (string, error) {
	if s.id == "" {
		return "", errors.New("not a driver")
	}
	return s.id, nil
}

func aLiveTrip() tracking.Session {
	return tracking.Session{ID: "sess-1", JobID: "job-1", DriverID: onTheWay,
		StartedAt: time.Now().Add(-5 * time.Minute)}
}

func aPosition(age time.Duration) tracking.Current {
	heading := 91.0
	speed := 8.5
	return tracking.Current{
		DriverID: onTheWay, Lat: 31.5204, Lon: 74.3587,
		HeadingDeg: &heading, SpeedMPS: &speed,
		RecordedAt: time.Now().Add(-age),
	}
}

func serveTracking(positions tracking.Positions, own string,
	roles ...identity.Role) *http.ServeMux {
	mux := http.NewServeMux()
	tracking.NewHandler(positions, stubOwnDriver{id: own}).Routes(mux,
		func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx := identity.ContextWithPrincipal(r.Context(),
					identity.Principal{UserID: watcherID, Roles: roles})
				next.ServeHTTP(w, r.WithContext(ctx))
			})
		})
	return mux
}

func get(t *testing.T, mux *http.ServeMux, target string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
	return recorder
}

func TestACustomerSeesTheDriverComingForThem(t *testing.T) {
	positions := &stubPositions{
		session: aLiveTrip(), live: true, current: aPosition(3 * time.Second), found: true,
	}
	response := get(t, serveTracking(positions, "", identity.RoleCustomer),
		"/api/v1/jobs/job-1/track")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", response.Code, response.Body)
	}
	var body tracking.TrackResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.DriverID != onTheWay || body.Latitude != 31.5204 {
		t.Fatalf("body = %+v", body)
	}
	if body.Stale {
		t.Error("a three-second-old position was reported stale")
	}
	if body.HeadingDeg == nil {
		t.Error("heading is what points the marker the right way")
	}
}

func TestAFrozenMarkerSaysSoRatherThanLying(t *testing.T) {
	// A marker that has not moved for minutes is a phone that lost signal, and
	// a customer watching it believes the driver is parked.
	positions := &stubPositions{
		session: aLiveTrip(), live: true, current: aPosition(6 * time.Minute), found: true,
	}
	response := get(t, serveTracking(positions, "", identity.RoleCustomer),
		"/api/v1/jobs/job-1/track")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	var body tracking.TrackResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Stale {
		t.Error("a six-minute-old position was presented as live")
	}
}

func TestAJobNobodyIsDrivingIsNotTracked(t *testing.T) {
	// The ordinary state of every job before it is accepted and after it ends.
	positions := &stubPositions{live: false}
	response := get(t, serveTracking(positions, "", identity.RoleCustomer),
		"/api/v1/jobs/job-1/track")

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", response.Code)
	}
	if len(positions.askedWith) != 0 {
		t.Error("permission was checked for a job that is not being tracked")
	}
}

func TestSomebodyElsesTripIsRefused(t *testing.T) {
	// Document 102's whole point. The refusal is already in the audit log by
	// the time this answers.
	positions := &stubPositions{
		session: aLiveTrip(), live: true, authErr: tracking.ErrNotPermitted,
	}
	response := get(t, serveTracking(positions, "", identity.RoleCustomer),
		"/api/v1/jobs/job-1/track")

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", response.Code)
	}
}

func TestTheScopeMatchesWhoIsAsking(t *testing.T) {
	// The scope decides which authorization query runs, so the wrong one is
	// either a leak or a locked-out customer.
	for _, testCase := range []struct {
		name      string
		roles     []identity.Role
		ownDriver string
		want      tracking.Scope
		wantRole  string
	}{
		{"customer", []identity.Role{identity.RoleCustomer}, "", tracking.ScopeOwnJob, "CUSTOMER"},
		{"the assigned driver", []identity.Role{identity.RoleDriver}, onTheWay,
			tracking.ScopeAssignedJob, "DRIVER"},
		// A driver looking at a trip that is not theirs is just a customer.
		{"another driver", []identity.Role{identity.RoleDriver}, "driver-99",
			tracking.ScopeOwnJob, "CUSTOMER"},
		{"support", []identity.Role{identity.RoleSupport}, "", tracking.ScopeOperations, "SUPPORT"},
		// An operator who is also a customer must be checked as the operator,
		// or the console refuses them their own job list.
		{"an admin who is also a customer",
			[]identity.Role{identity.RoleCustomer, identity.RoleAdmin}, "",
			tracking.ScopeOperations, "ADMIN"},
	} {
		positions := &stubPositions{
			session: aLiveTrip(), live: true, current: aPosition(time.Second), found: true,
		}
		get(t, serveTracking(positions, testCase.ownDriver, testCase.roles...),
			"/api/v1/jobs/job-1/track")

		if len(positions.askedWith) != 1 {
			t.Fatalf("%s: asked %d times", testCase.name, len(positions.askedWith))
		}
		asked := positions.askedWith[0]
		if asked.scope != testCase.want {
			t.Errorf("%s: scope = %s, want %s", testCase.name, asked.scope, testCase.want)
		}
		if asked.role != testCase.wantRole {
			t.Errorf("%s: role = %q, want %q", testCase.name, asked.role, testCase.wantRole)
		}
		if asked.driverID != onTheWay || asked.jobID != "job-1" {
			t.Errorf("%s: asked about %s on %s", testCase.name, asked.driverID, asked.jobID)
		}
	}
}

func TestAnUnknownPositionIsNotAnUntrackedTrip(t *testing.T) {
	// The trip is live and the driver's phone has said nothing yet — a shift
	// starting indoors, a dead spot. A client should keep asking.
	positions := &stubPositions{session: aLiveTrip(), live: true, found: false}
	response := get(t, serveTracking(positions, "", identity.RoleCustomer),
		"/api/v1/jobs/job-1/track")

	if response.Code == http.StatusNotFound {
		t.Fatal("an unknown position was reported as an untracked trip")
	}
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", response.Code)
	}
}
