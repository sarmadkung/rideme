// Package driver is the driver-facing API surface.
//
// The domain logic it exposes already existed and was tested — availability is
// a state machine in providers, location validation is in tracking, and the
// trip commands are in booking. What was missing was any way for a driver's
// phone to reach it: a driver could be sent an offer but had no endpoint to
// learn about one, no way to go online, and no way to report a position.
//
// This package composes those three, and adds no rules of its own beyond the
// ones that only make sense at the boundary — chiefly that going online and
// joining the dispatch pool must happen together.
package driver

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sarmadkung/rideme/services/api/internal/jobs"
	"github.com/sarmadkung/rideme/services/api/internal/providers"
	"github.com/sarmadkung/rideme/services/api/internal/tracking"
	"github.com/sarmadkung/rideme/services/api/pkg/httpx"
)

// Service is the driver's view of the platform.
type Service struct {
	providers *providers.Store
	tracking  *tracking.Store
	jobs      *jobs.Store
	limits    tracking.Limits
	now       func() time.Time
}

func NewService(providerStore *providers.Store, trackingStore *tracking.Store,
	jobStore *jobs.Store, limits tracking.Limits, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	if limits == (tracking.Limits{}) {
		limits = tracking.DefaultLimits()
	}
	return &Service{providers: providerStore, tracking: trackingStore,
		jobs: jobStore, limits: limits, now: now}
}

var (
	// ErrNoVehicle reports a driver trying to go online with no active
	// vehicle. Dispatch matches jobs to vehicle capabilities, so a driver
	// without one cannot be offered anything and would sit online forever
	// wondering why.
	ErrNoVehicle = errors.New("driver: no active vehicle is selected")
	// ErrNotADriver reports a user who has no driver record.
	ErrNotADriver = errors.New("driver: this account is not a driver")
)

// Me returns the driver record behind an authenticated user.
func (s *Service) Me(ctx context.Context, userID string) (providers.Driver, error) {
	d, err := s.providers.DriverByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, providers.ErrDriverNotFound) {
			return providers.Driver{}, ErrNotADriver
		}
		return providers.Driver{}, err
	}
	return d, nil
}

// GoOnline makes a driver available for dispatch.
//
// Two things must happen together: the driver's status becomes AVAILABLE, and
// their position enters the dispatch pool. Doing only the first produces a
// driver who believes they are working and is never offered anything, because
// the geo search that finds candidates reads the pool, not the column.
//
// The position is required for the same reason. "Online" without a location is
// not a state dispatch can use.
func (s *Service) GoOnline(ctx context.Context, userID string, at tracking.Fix) (providers.Driver, error) {
	d, err := s.Me(ctx, userID)
	if err != nil {
		return providers.Driver{}, err
	}
	if d.ActiveVehicleID == "" {
		return providers.Driver{}, ErrNoVehicle
	}

	at.DriverID = d.ID
	at.VehicleID = d.ActiveVehicleID
	if at.RecordedAt.IsZero() {
		at.RecordedAt = s.now()
	}
	if err := tracking.Validate(at, nil, s.limits, s.now()); err != nil {
		return providers.Driver{}, err
	}

	updated, err := s.providers.TransitionAvailability(ctx, d.ID, d.Status, providers.StatusAvailable)
	if err != nil {
		return providers.Driver{}, err
	}
	if err := s.tracking.PutCurrent(ctx, currentFrom(at), true); err != nil {
		// The status moved but the pool did not. Rolling the status back is
		// the honest outcome: leaving it AVAILABLE would show the driver as
		// working while dispatch cannot see them.
		_, _ = s.providers.TransitionAvailability(ctx, d.ID, providers.StatusAvailable, d.Status)
		return providers.Driver{}, fmt.Errorf("join dispatch pool: %w", err)
	}
	return updated, nil
}

// GoOffline withdraws a driver from dispatch.
//
// The pool is left first. If the status update then fails, the driver is
// removed from dispatch but still reads as AVAILABLE — which is a stale label,
// not a driver receiving offers they will not answer. The reverse order would
// leave them in the pool believing they had signed off.
func (s *Service) GoOffline(ctx context.Context, userID string) (providers.Driver, error) {
	d, err := s.Me(ctx, userID)
	if err != nil {
		return providers.Driver{}, err
	}
	if err := s.tracking.RemoveFromPool(ctx, d.ID); err != nil {
		return providers.Driver{}, fmt.Errorf("leave dispatch pool: %w", err)
	}
	return s.providers.TransitionAvailability(ctx, d.ID, d.Status, providers.StatusOffline)
}

// ReportLocation records a batch of position fixes.
//
// Batching is document 048's model: a phone buffers while offline and sends
// what it has. Each fix is validated against the one before it, so a rejected
// fix does not become the baseline the next one is compared to.
//
// Rejections are counted rather than raised. A driver whose phone reported one
// bad coordinate in a batch of twenty should not have the other nineteen
// dropped, and should not see an error for something they cannot act on.
func (s *Service) ReportLocation(ctx context.Context, userID string, fixes []tracking.Fix) (accepted int, rejected []RejectedFix, err error) {
	d, err := s.Me(ctx, userID)
	if err != nil {
		return 0, nil, err
	}

	previous, err := s.tracking.LastFix(ctx, d.ID)
	if err != nil {
		return 0, nil, err
	}

	now := s.now()
	valid := make([]tracking.Fix, 0, len(fixes))
	for _, fix := range fixes {
		fix.DriverID = d.ID
		if fix.VehicleID == "" {
			fix.VehicleID = d.ActiveVehicleID
		}
		if verr := tracking.Validate(fix, previous, s.limits, now); verr != nil {
			var rej *tracking.ErrRejected
			if errors.As(verr, &rej) {
				rejected = append(rejected, RejectedFix{
					RecordedAt: fix.RecordedAt, Reason: rej.Reason, Detail: rej.Detail,
				})
				continue
			}
			return 0, nil, verr
		}
		valid = append(valid, fix)
		previous = &tracking.Previous{Lat: fix.Lat, Lon: fix.Lon, RecordedAt: fix.RecordedAt}
	}

	if len(valid) == 0 {
		return 0, rejected, nil
	}
	if _, err := s.tracking.AppendHistory(ctx, valid); err != nil {
		return 0, nil, err
	}

	// The newest accepted fix becomes the live position. Only a driver who is
	// available belongs in the dispatch pool — one who is mid-trip still
	// reports position for the customer to follow, but must not be offered
	// another job.
	latest := valid[len(valid)-1]
	if err := s.tracking.PutCurrent(ctx, currentFrom(latest), d.Status == providers.StatusAvailable); err != nil {
		return 0, nil, err
	}
	return len(valid), rejected, nil
}

// RejectedFix reports one fix the pipeline discarded and why, so a driver app
// can tell "we dropped your GPS spike" from "we lost your report".
type RejectedFix struct {
	RecordedAt time.Time          `json:"recorded_at"`
	Reason     tracking.Rejection `json:"reason"`
	Detail     string             `json:"detail,omitempty"`
}

// Assignment is the offer or trip a driver currently holds, with the job it
// refers to.
type Assignment struct {
	Assignment jobs.Assignment
	Job        jobs.Job
}

// Current returns what the driver should be looking at.
//
// httpx.NotFound when there is nothing: an idle driver is not an error, and
// the app polls this constantly.
func (s *Service) Current(ctx context.Context, userID string) (Assignment, error) {
	d, err := s.Me(ctx, userID)
	if err != nil {
		return Assignment{}, err
	}
	assignment, err := s.jobs.LiveAssignmentForDriver(ctx, d.ID)
	if err != nil {
		if errors.Is(err, jobs.ErrNotFound) {
			return Assignment{}, httpx.NotFound("no current assignment")
		}
		return Assignment{}, err
	}
	job, err := s.jobs.ByID(ctx, assignment.JobID)
	if err != nil {
		return Assignment{}, err
	}
	return Assignment{Assignment: assignment, Job: job}, nil
}

func currentFrom(f tracking.Fix) tracking.Current {
	return tracking.Current{
		DriverID: f.DriverID, VehicleID: f.VehicleID, JobID: f.JobID,
		Lat: f.Lat, Lon: f.Lon,
		HeadingDeg: f.HeadingDeg, SpeedMPS: f.SpeedMPS,
		RecordedAt: f.RecordedAt,
	}
}
