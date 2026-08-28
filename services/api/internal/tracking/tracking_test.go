package tracking_test

import (
	"testing"
	"time"

	"github.com/sarmadkung/rideme/services/api/internal/tracking"
)

var now = time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)

func goodFix() tracking.Fix {
	accuracy, heading, speed := 8.0, 90.0, 12.0
	return tracking.Fix{
		DriverID:   "driver-1",
		Lat:        31.5204,
		Lon:        74.3587,
		AccuracyM:  &accuracy,
		HeadingDeg: &heading,
		SpeedMPS:   &speed,
		RecordedAt: now.Add(-5 * time.Second),
	}
}

func reasonOf(t *testing.T, err error) tracking.Rejection {
	t.Helper()
	rejected, ok := err.(*tracking.ErrRejected)
	if !ok {
		t.Fatalf("want a rejection, got %v", err)
	}
	return rejected.Reason
}

func TestAGoodFixIsAccepted(t *testing.T) {
	if err := tracking.Validate(goodFix(), nil, tracking.DefaultLimits(), now); err != nil {
		t.Fatalf("a valid fix was rejected: %v", err)
	}
}

func TestDocument48ValidationChecks(t *testing.T) {
	// The list document 48 requires: impossible coordinates, future
	// timestamps, stale timestamps, impossible speed, unrealistic jumps.
	limits := tracking.DefaultLimits()

	cases := []struct {
		name   string
		mutate func(*tracking.Fix)
		want   tracking.Rejection
	}{
		{"impossible latitude", func(f *tracking.Fix) { f.Lat = 91 }, tracking.RejectBadCoordinate},
		{"impossible longitude", func(f *tracking.Fix) { f.Lon = -181 }, tracking.RejectBadCoordinate},
		{"null island", func(f *tracking.Fix) { f.Lat, f.Lon = 0, 0 }, tracking.RejectBadCoordinate},
		{"future timestamp", func(f *tracking.Fix) { f.RecordedAt = now.Add(5 * time.Minute) }, tracking.RejectFutureTimestamp},
		{"stale timestamp", func(f *tracking.Fix) { f.RecordedAt = now.Add(-time.Hour) }, tracking.RejectStaleTimestamp},
		{"no timestamp", func(f *tracking.Fix) { f.RecordedAt = time.Time{} }, tracking.RejectStaleTimestamp},
		{"impossible speed", func(f *tracking.Fix) { s := 200.0; f.SpeedMPS = &s }, tracking.RejectImpossibleSpeed},
		{"useless accuracy", func(f *tracking.Fix) { a := 5000.0; f.AccuracyM = &a }, tracking.RejectPoorAccuracy},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fix := goodFix()
			tc.mutate(&fix)
			err := tracking.Validate(fix, nil, limits, now)
			if err == nil {
				t.Fatal("the fix was accepted")
			}
			if got := reasonOf(t, err); got != tc.want {
				t.Fatalf("reason = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestSmallClockSkewIsTolerated(t *testing.T) {
	// Phone clocks drift. Rejecting a fix a few seconds ahead would discard
	// good data from a large share of devices.
	fix := goodFix()
	fix.RecordedAt = now.Add(10 * time.Second)
	if err := tracking.Validate(fix, nil, tracking.DefaultLimits(), now); err != nil {
		t.Fatalf("a slightly fast clock was rejected: %v", err)
	}
}

func TestAnImpossibleJumpIsRejected(t *testing.T) {
	// Lahore to Gujranwala is ~67km. Covering it in ten seconds is a spoof or
	// a bug, and dispatching against it would send a driver who is not there.
	previous := &tracking.Previous{Lat: 31.5880, Lon: 74.3150, RecordedAt: now.Add(-10 * time.Second)}
	fix := goodFix()
	fix.Lat, fix.Lon = 32.1877, 74.1945
	fix.RecordedAt = now

	err := tracking.Validate(fix, previous, tracking.DefaultLimits(), now)
	if got := reasonOf(t, err); got != tracking.RejectImpossibleJump {
		t.Fatalf("reason = %s, want impossible_jump", got)
	}
}

func TestAPlausibleMovementIsAccepted(t *testing.T) {
	// ~500m in 30 seconds is 60 km/h — fast, legal, and must not be rejected.
	previous := &tracking.Previous{Lat: 31.5204, Lon: 74.3587, RecordedAt: now.Add(-30 * time.Second)}
	fix := goodFix()
	fix.Lat, fix.Lon = 31.5249, 74.3587
	fix.RecordedAt = now

	if err := tracking.Validate(fix, previous, tracking.DefaultLimits(), now); err != nil {
		t.Fatalf("normal movement was rejected: %v", err)
	}
}

func TestALongGapAllowsALargeMove(t *testing.T) {
	// A driver who loses signal in a tunnel reappears far away. Rejecting that
	// would blind dispatch to a driver who is simply back on the network.
	previous := &tracking.Previous{Lat: 31.5204, Lon: 74.3587, RecordedAt: now.Add(-4 * time.Minute)}
	fix := goodFix()
	fix.Lat, fix.Lon = 31.5880, 74.3150
	fix.RecordedAt = now

	if err := tracking.Validate(fix, previous, tracking.DefaultLimits(), now); err != nil {
		t.Fatalf("a legitimate long gap was rejected: %v", err)
	}
}

func TestOutOfOrderFixesAreDropped(t *testing.T) {
	// Buffered delivery reorders packets. An older fix is not news, and
	// applying it would move the driver backwards.
	previous := &tracking.Previous{Lat: 31.5204, Lon: 74.3587, RecordedAt: now.Add(-5 * time.Second)}
	fix := goodFix()
	fix.RecordedAt = now.Add(-20 * time.Second)

	err := tracking.Validate(fix, previous, tracking.DefaultLimits(), now)
	if got := reasonOf(t, err); got != tracking.RejectOutOfOrder {
		t.Fatalf("reason = %s, want out_of_order", got)
	}
}

func TestAFixIdenticalInTimeIsAlsoDropped(t *testing.T) {
	previous := &tracking.Previous{Lat: 31.52, Lon: 74.35, RecordedAt: now}
	fix := goodFix()
	fix.RecordedAt = now
	if err := tracking.Validate(fix, previous, tracking.DefaultLimits(), now); err == nil {
		t.Fatal("a duplicate timestamp was accepted")
	}
}

func TestFreshnessDecidesDispatchVisibility(t *testing.T) {
	// Document 16: dispatch excludes stale locations.
	current := tracking.Current{RecordedAt: now.Add(-30 * time.Second)}
	if !current.Fresh(now, time.Minute) {
		t.Error("a 30-second-old position was considered stale")
	}
	if current.Fresh(now, 10*time.Second) {
		t.Error("a 30-second-old position passed a 10-second freshness bar")
	}
}

func TestRejectionsCarryAReadableReason(t *testing.T) {
	fix := goodFix()
	fix.Lat = 91
	err := tracking.Validate(fix, nil, tracking.DefaultLimits(), now)
	if err == nil {
		t.Fatal("expected an error")
	}
	// An operator investigating why a driver vanished needs the reason, not
	// just that something was dropped.
	if msg := err.Error(); msg == "" || msg == "tracking: " {
		t.Fatalf("unhelpful message %q", msg)
	}
}

func TestSessionLiveness(t *testing.T) {
	ended := now
	if !(tracking.Session{}).Live() {
		t.Error("a session with no end time is live")
	}
	if (tracking.Session{EndedAt: &ended}).Live() {
		t.Error("an ended session is not live")
	}
}
