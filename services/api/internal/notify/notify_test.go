package notify

import "testing"

// Document 124: some messages "cannot be disabled when required for security,
// payment, active service, safety, legal/transactional requirements".
//
// The rule is a function rather than a column so it cannot be edited into
// nothing by a row update, and this test is what says which categories it
// covers. A customer who could mute RIDE would sit waiting for a driver who
// arrived ten minutes ago.
func TestRequiredCategoriesCannotBeDisabled(t *testing.T) {
	required := map[Category]bool{
		CategorySafety: true, CategoryPayment: true,
		CategoryRide: true, CategoryDelivery: true, CategoryOrder: true,
		CategorySupport: false, CategoryMarketing: false,
	}
	for category, want := range required {
		if got := category.Required(); got != want {
			t.Errorf("%s required = %v, want %v", category, got, want)
		}
	}
}

// A customer muting grocery updates must not thereby mute their ride, which
// is what one category for every service would mean.
func TestEachServiceHasItsOwnCategory(t *testing.T) {
	cases := map[string]Category{
		"RIDE":    CategoryRide,
		"GROCERY": CategoryOrder,
		"PARCEL":  CategoryDelivery,
		"CARGO":   CategoryDelivery,
		"FREIGHT": CategoryDelivery,
		"grocery": CategoryOrder, // the mapping is case-insensitive
		"":        CategoryRide,  // and never returns an invalid category
	}
	for jobType, want := range cases {
		if got := categoryForJobType(jobType); got != want {
			t.Errorf("job type %q maps to %s, want %s", jobType, got, want)
		}
	}
}

// Being buzzed for every internal transition is how people learn to turn
// notifications off, which then costs them the arrival they did want. ASSIGNED
// is the case that matters: the driver has not accepted and the assignment can
// still move to somebody else.
func TestOnlyStatusesWorthABuzzAreNotified(t *testing.T) {
	for _, status := range []string{"ACCEPTED", "AT_PICKUP", "COMPLETED", "CANCELLED"} {
		if _, ok := jobCopy[status]; !ok {
			t.Errorf("%s produces no notification and should", status)
		}
	}
	for _, status := range []string{"DRAFT", "QUOTED", "REQUESTED", "SEARCHING", "ASSIGNED", "AT_DROPOFF"} {
		if _, ok := jobCopy[status]; ok {
			t.Errorf("%s produces a notification and should not", status)
		}
	}
}

// Every category and channel the database will accept must be a value this
// package considers valid, and nothing else. A mismatch means either a write
// that fails a CHECK constraint at runtime or a value that bypasses the
// preference rule.
func TestOnlyTheDocumentedCategoriesAndChannelsAreValid(t *testing.T) {
	for _, c := range []Category{CategoryRide, CategoryDelivery, CategoryOrder,
		CategoryPayment, CategorySafety, CategorySupport, CategoryMarketing} {
		if !c.Valid() {
			t.Errorf("%s is not valid and must be", c)
		}
	}
	if Category("PROMOTIONS").Valid() {
		t.Error("an undocumented category is valid")
	}
	for _, ch := range []Channel{ChannelPush, ChannelSMS, ChannelEmail, ChannelInApp} {
		if !ch.Valid() {
			t.Errorf("%s is not valid and must be", ch)
		}
	}
	if Channel("CARRIER_PIGEON").Valid() {
		t.Error("an undocumented channel is valid")
	}
	if !PlatformIOS.Valid() || !PlatformAndroid.Valid() || !PlatformWeb.Valid() {
		t.Error("a documented platform is not valid")
	}
	if Platform("SYMBIAN").Valid() {
		t.Error("an undocumented platform is valid")
	}
}

// One event must produce one buzz per device, however many times the
// transition that produced it is retried.
func TestTheIdempotencyKeyIsPerJobAndStatus(t *testing.T) {
	const jobID = "job-1"
	first := "job:" + jobID + ":ACCEPTED"
	if got := keyFor(jobID, "ACCEPTED"); got != first {
		t.Errorf("key %q, want %q", got, first)
	}
	if keyFor(jobID, "ACCEPTED") == keyFor(jobID, "COMPLETED") {
		t.Error("two different statuses share an idempotency key")
	}
	if keyFor("job-1", "ACCEPTED") == keyFor("job-2", "ACCEPTED") {
		t.Error("two different jobs share an idempotency key")
	}
}
