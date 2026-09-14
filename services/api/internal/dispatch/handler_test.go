package dispatch_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sarmadkung/rideme/services/api/internal/dispatch"
	"github.com/sarmadkung/rideme/services/api/internal/identity"
	"github.com/sarmadkung/rideme/services/api/internal/jobs"
	"github.com/sarmadkung/rideme/services/api/internal/providers"
	"github.com/sarmadkung/rideme/services/api/internal/tracking"
)

const (
	riderUserID = "user-driver-1"
	riderID     = "driver-1"
)

type stubOffers struct {
	assignment jobs.Assignment
	acceptErr  error
	rejectErr  error
	accepted   []string
	rejected   []string
}

func (s *stubOffers) Accept(_ context.Context, jobID, driverID string) (jobs.Assignment, error) {
	if s.acceptErr != nil {
		return jobs.Assignment{}, s.acceptErr
	}
	s.accepted = append(s.accepted, jobID+"/"+driverID)
	a := s.assignment
	a.JobID, a.DriverID = jobID, driverID
	return a, nil
}

func (s *stubOffers) Reject(_ context.Context, jobID, driverID string) error {
	if s.rejectErr != nil {
		return s.rejectErr
	}
	s.rejected = append(s.rejected, jobID+"/"+driverID)
	return nil
}

type stubDrivers struct {
	id  string
	err error
}

func (s stubDrivers) DriverByUserID(_ context.Context, _ string) (providers.Driver, error) {
	if s.err != nil {
		return providers.Driver{}, s.err
	}
	return providers.Driver{ID: s.id}, nil
}

type stubPresence struct {
	removed  []string
	sessions []string
	poolErr  error
	trackErr error
}

func (s *stubPresence) RemoveFromPool(_ context.Context, driverID string) error {
	if s.poolErr != nil {
		return s.poolErr
	}
	s.removed = append(s.removed, driverID)
	return nil
}

func (s *stubPresence) StartSession(_ context.Context, jobID, driverID string) (tracking.Session, error) {
	if s.trackErr != nil {
		return tracking.Session{}, s.trackErr
	}
	s.sessions = append(s.sessions, jobID+"/"+driverID)
	return tracking.Session{JobID: jobID, DriverID: driverID}, nil
}

func serveOffers(offers dispatch.Offers, presence dispatch.Presence,
	roles ...identity.Role) *http.ServeMux {
	mux := http.NewServeMux()
	handler := dispatch.NewHandler(offers, stubDrivers{id: riderID}, presence,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	handler.Routes(mux, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := identity.ContextWithPrincipal(r.Context(),
				identity.Principal{UserID: riderUserID, Roles: roles})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	return mux
}

func post(t *testing.T, mux *http.ServeMux, target string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, target, strings.NewReader("")))
	return recorder
}

func TestAcceptingAnOfferAnswersWithTheAssignment(t *testing.T) {
	// Before this route existed, accept answered 503 pointing at a dispatch
	// surface nobody had built: the app could see an offer and not take it.
	offers := &stubOffers{assignment: jobs.Assignment{ID: "assign-1", Status: jobs.AssignmentAccepted}}
	presence := &stubPresence{}
	response := post(t, serveOffers(offers, presence, identity.RoleDriver),
		"/api/v1/driver/jobs/job-1/accept")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", response.Code, response.Body)
	}
	var body dispatch.AcceptResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.AssignmentID != "assign-1" || body.JobID != "job-1" {
		t.Fatalf("body = %+v", body)
	}
	if len(offers.accepted) != 1 || offers.accepted[0] != "job-1/"+riderID {
		t.Fatalf("accepted = %+v", offers.accepted)
	}
}

func TestAcceptingLeavesThePoolAndOpensTracking(t *testing.T) {
	// Accept's own comment: the geo index is cleared by the caller. And the
	// tracking session is what lets the customer watch this driver, for this
	// job, while it is live.
	offers := &stubOffers{}
	presence := &stubPresence{}
	post(t, serveOffers(offers, presence, identity.RoleDriver), "/api/v1/driver/jobs/job-1/accept")

	if len(presence.removed) != 1 || presence.removed[0] != riderID {
		t.Errorf("an accepted driver is still in the geo pool: %+v", presence.removed)
	}
	if len(presence.sessions) != 1 || presence.sessions[0] != "job-1/"+riderID {
		t.Errorf("tracking was not opened: %+v", presence.sessions)
	}
}

func TestAnAcceptedJobIsStillWonWhenTrackingFails(t *testing.T) {
	// Both side effects follow a committed acceptance. Failing the request
	// would have the driver tap again on a job they already hold.
	offers := &stubOffers{}
	presence := &stubPresence{poolErr: errors.New("redis down"), trackErr: errors.New("redis down")}
	response := post(t, serveOffers(offers, presence, identity.RoleDriver),
		"/api/v1/driver/jobs/job-1/accept")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d; the acceptance had already committed", response.Code)
	}
}

func TestLosingTheRaceIsAConflictNotAFailure(t *testing.T) {
	for _, testCase := range []struct {
		err  error
		want int
	}{
		{dispatch.ErrOfferNotFound, http.StatusConflict},
		{dispatch.ErrReservationLost, http.StatusConflict},
		{dispatch.ErrJobClaimed, http.StatusConflict},
		// Not a race: this driver was suspended between the offer and the tap,
		// and telling them somebody else was faster would be a lie.
		{dispatch.ErrNotEligible, http.StatusForbidden},
	} {
		offers := &stubOffers{acceptErr: testCase.err}
		response := post(t, serveOffers(offers, &stubPresence{}, identity.RoleDriver),
			"/api/v1/driver/jobs/job-1/accept")
		if response.Code != testCase.want {
			t.Errorf("%v: status = %d, want %d", testCase.err, response.Code, testCase.want)
		}
	}
}

func TestALostRaceOpensNoTracking(t *testing.T) {
	offers := &stubOffers{acceptErr: dispatch.ErrJobClaimed}
	presence := &stubPresence{}
	post(t, serveOffers(offers, presence, identity.RoleDriver), "/api/v1/driver/jobs/job-1/accept")

	if len(presence.sessions) != 0 || len(presence.removed) != 0 {
		t.Errorf("a driver who did not win the job was taken out of the pool: %+v", presence)
	}
}

func TestRejectingReturnsNothing(t *testing.T) {
	// The job has gone back to dispatch and nothing about it is still the
	// driver's, so there is nothing to return.
	offers := &stubOffers{}
	response := post(t, serveOffers(offers, &stubPresence{}, identity.RoleDriver),
		"/api/v1/driver/jobs/job-1/reject")

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", response.Code)
	}
	if len(offers.rejected) != 1 {
		t.Fatalf("rejected = %+v", offers.rejected)
	}
}

func TestRejectingAnOfferThatIsGoneIsAConflict(t *testing.T) {
	offers := &stubOffers{rejectErr: dispatch.ErrOfferNotFound}
	response := post(t, serveOffers(offers, &stubPresence{}, identity.RoleDriver),
		"/api/v1/driver/jobs/job-1/reject")

	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, body %s", response.Code, response.Body)
	}
}

func TestACustomerCannotAnswerAnOffer(t *testing.T) {
	offers := &stubOffers{}
	response := post(t, serveOffers(offers, &stubPresence{}, identity.RoleCustomer),
		"/api/v1/driver/jobs/job-1/accept")

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", response.Code)
	}
	if len(offers.accepted) != 0 {
		t.Errorf("accepted = %+v", offers.accepted)
	}
}

func TestAnAccountWithNoDriverRecordIsRefused(t *testing.T) {
	mux := http.NewServeMux()
	offers := &stubOffers{}
	dispatch.NewHandler(offers, stubDrivers{err: errors.New("no driver")}, &stubPresence{},
		slog.New(slog.NewTextHandler(io.Discard, nil))).
		Routes(mux, func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx := identity.ContextWithPrincipal(r.Context(), identity.Principal{
					UserID: riderUserID, Roles: []identity.Role{identity.RoleDriver},
				})
				next.ServeHTTP(w, r.WithContext(ctx))
			})
		})

	response := post(t, mux, "/api/v1/driver/jobs/job-1/accept")
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", response.Code)
	}
	if len(offers.accepted) != 0 {
		t.Errorf("accepted = %+v", offers.accepted)
	}
}
