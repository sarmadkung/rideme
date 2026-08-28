package jobs_test

import (
	"testing"

	"github.com/sarmadkung/rideme/services/api/internal/jobs"
)

// The happy path document 15 draws, walked end to end. If this breaks, the
// documented lifecycle no longer works.
func TestTheDocumentedMainFlowIsWalkable(t *testing.T) {
	flow := []jobs.Status{
		jobs.StatusDraft, jobs.StatusQuoted, jobs.StatusRequested, jobs.StatusSearching,
		jobs.StatusAssigned, jobs.StatusAccepted, jobs.StatusArriving, jobs.StatusAtPickup,
		jobs.StatusInProgress, jobs.StatusAtDropoff, jobs.StatusCompleted,
	}
	for i := 0; i < len(flow)-1; i++ {
		if err := jobs.Machine.Validate(flow[i], flow[i+1]); err != nil {
			t.Fatalf("%s -> %s should be allowed: %v", flow[i], flow[i+1], err)
		}
	}
}

func TestTerminalStatesAreTerminal(t *testing.T) {
	// Document 15 names four. A terminal state with a way out is not terminal,
	// and every consumer that asks "is this finished?" would be wrong.
	for _, status := range []jobs.Status{
		jobs.StatusCancelled, jobs.StatusFailed, jobs.StatusExpired, jobs.StatusDisputed,
	} {
		if !jobs.Machine.Terminal(status) {
			t.Errorf("%s should be terminal", status)
		}
		if next := jobs.Machine.Next(status); len(next) != 0 {
			t.Errorf("%s is terminal but leads to %v", status, next)
		}
	}
}

func TestStatusCannotSkipAheadOrGoBackwards(t *testing.T) {
	// The transitions a buggy client would attempt.
	refused := []struct{ from, to jobs.Status }{
		{jobs.StatusDraft, jobs.StatusCompleted},     // straight to done
		{jobs.StatusDraft, jobs.StatusInProgress},    // skipping dispatch entirely
		{jobs.StatusRequested, jobs.StatusAccepted},  // accepted with nobody assigned
		{jobs.StatusInProgress, jobs.StatusDraft},    // backwards
		{jobs.StatusCompleted, jobs.StatusCancelled}, // un-completing a finished job
		{jobs.StatusCancelled, jobs.StatusRequested}, // resurrecting a cancelled job
		{jobs.StatusAtDropoff, jobs.StatusAtPickup},  // backwards along the route
	}
	for _, tc := range refused {
		if err := jobs.Machine.Validate(tc.from, tc.to); err == nil {
			t.Errorf("%s -> %s should be refused", tc.from, tc.to)
		}
	}
}

func TestCancellationIsReachableWhileTheJobIsStillLive(t *testing.T) {
	// A customer can call off a job before it finishes. What that costs is
	// BD-01 and unresolved; that it is possible is not.
	for _, from := range []jobs.Status{
		jobs.StatusDraft, jobs.StatusQuoted, jobs.StatusRequested, jobs.StatusSearching,
		jobs.StatusAssigned, jobs.StatusAccepted, jobs.StatusArriving, jobs.StatusAtPickup,
	} {
		if err := jobs.Machine.Validate(from, jobs.StatusCancelled); err != nil {
			t.Errorf("cancelling from %s should be allowed: %v", from, err)
		}
	}
}

func TestADeclinedOfferReturnsTheJobToSearching(t *testing.T) {
	// Dispatch reassignment (documents 44, 45): a driver who rejects or times
	// out must not strand the job.
	for _, from := range []jobs.Status{jobs.StatusAssigned, jobs.StatusAccepted} {
		if err := jobs.Machine.Validate(from, jobs.StatusSearching); err != nil {
			t.Errorf("%s -> SEARCHING should be allowed for reassignment: %v", from, err)
		}
	}
}

func TestTransitionErrorNamesWhatWasAllowed(t *testing.T) {
	err := jobs.Machine.Validate(jobs.StatusDraft, jobs.StatusCompleted)
	if err == nil {
		t.Fatal("expected an error")
	}
	// An error that only says "invalid" makes every caller guess.
	if msg := err.Error(); !contains(msg, "QUOTED") || !contains(msg, "job") {
		t.Fatalf("error should name the machine and the allowed states, got %q", msg)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}

func TestAssignmentLifecycle(t *testing.T) {
	if err := jobs.AssignmentMachine.Validate(jobs.AssignmentOffered, jobs.AssignmentAccepted); err != nil {
		t.Fatal(err)
	}
	if err := jobs.AssignmentMachine.Validate(jobs.AssignmentAccepted, jobs.AssignmentCompleted); err != nil {
		t.Fatal(err)
	}
	// An expired or rejected offer is over. Re-offering is a new assignment
	// row, which is what keeps the dispatch history readable.
	for _, from := range []jobs.AssignmentStatus{
		jobs.AssignmentRejected, jobs.AssignmentExpired, jobs.AssignmentCancelled, jobs.AssignmentCompleted,
	} {
		if !jobs.AssignmentMachine.Terminal(from) {
			t.Errorf("%s should be terminal", from)
		}
	}
	if err := jobs.AssignmentMachine.Validate(jobs.AssignmentRejected, jobs.AssignmentAccepted); err == nil {
		t.Error("a rejected offer was accepted")
	}
}

func TestJobTypeIsClosed(t *testing.T) {
	for _, valid := range jobs.AllTypes {
		if !valid.Valid() {
			t.Errorf("%s should be valid", valid)
		}
	}
	if jobs.Type("TAXI").Valid() {
		t.Error("an undocumented job type was accepted")
	}
	if len(jobs.AllTypes) != 5 {
		t.Fatalf("document 04 fixes five job types, found %d", len(jobs.AllTypes))
	}
}

func TestCoordinateValidation(t *testing.T) {
	valid := jobs.Coordinate{Latitude: 31.5204, Longitude: 74.3587} // Lahore
	if !valid.Valid() {
		t.Error("a real coordinate was rejected")
	}
	for _, bad := range []jobs.Coordinate{
		{Latitude: 91, Longitude: 0},
		{Latitude: -91, Longitude: 0},
		{Latitude: 0, Longitude: 181},
		{Latitude: 0, Longitude: -181},
		{Latitude: 0, Longitude: 0}, // unset coordinates, not the Gulf of Guinea
	} {
		if bad.Valid() {
			t.Errorf("%+v should be rejected", bad)
		}
	}
}

func TestJobValidationRefusesAnUnusableJob(t *testing.T) {
	base := func() jobs.Job {
		return jobs.Job{
			Type:            jobs.TypeRide,
			RequesterUserID: "user-1",
			Stops: []jobs.Stop{
				{Sequence: 0, Type: jobs.StopPickup, Location: jobs.Coordinate{Latitude: 31.52, Longitude: 74.35}},
				{Sequence: 1, Type: jobs.StopDropoff, Location: jobs.Coordinate{Latitude: 31.58, Longitude: 74.32}},
			},
		}
	}
	if err := base().ValidateForCreation(); err != nil {
		t.Fatalf("a well-formed job was rejected: %v", err)
	}

	noStops := base()
	noStops.Stops = nil
	if err := noStops.ValidateForCreation(); err == nil {
		t.Error("a job with no stops was accepted")
	}

	badType := base()
	badType.Type = "TAXI"
	if err := badType.ValidateForCreation(); err == nil {
		t.Error("an undocumented type was accepted")
	}

	// A gap in the sequence means a stop went missing between the client and
	// here, and the route is not what anyone thinks it is.
	gapped := base()
	gapped.Stops[1].Sequence = 5
	if err := gapped.ValidateForCreation(); err == nil {
		t.Error("a job with a gap in its stop sequence was accepted")
	}

	duplicated := base()
	duplicated.Stops[1].Sequence = 0
	if err := duplicated.ValidateForCreation(); err == nil {
		t.Error("a job with duplicate stop sequences was accepted")
	}
}

func TestPickupAndDropoffOnAMultiStopJob(t *testing.T) {
	job := jobs.Job{Stops: []jobs.Stop{
		{Sequence: 0, Type: jobs.StopPickup, Address: "origin"},
		{Sequence: 1, Type: jobs.StopDropoff, Address: "first drop"},
		{Sequence: 2, Type: jobs.StopDropoff, Address: "final drop"},
	}}
	pickup, ok := job.Pickup()
	if !ok || pickup.Address != "origin" {
		t.Fatalf("pickup = %+v", pickup)
	}
	// The last dropoff is the destination — multi-stop delivery (document 82)
	// depends on this not returning the first one.
	dropoff, ok := job.Dropoff()
	if !ok || dropoff.Address != "final drop" {
		t.Fatalf("dropoff = %+v", dropoff)
	}
}

func TestLiveReflectsTerminality(t *testing.T) {
	if !(jobs.Job{Status: jobs.StatusInProgress}).Live() {
		t.Error("an in-progress job is live")
	}
	if (jobs.Job{Status: jobs.StatusCompleted}).Live() {
		t.Error("a completed job is not live")
	}
	if (jobs.Job{Status: jobs.StatusCancelled}).Live() {
		t.Error("a cancelled job is not live")
	}
}
