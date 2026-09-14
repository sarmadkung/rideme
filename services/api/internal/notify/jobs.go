package notify

import (
	"context"
	"fmt"
	"strings"
)

// JobNotifier turns a job's lifecycle into messages a person understands.
//
// It lives here rather than in booking for document 121's reason: the wording,
// the category and the deep link are communication concerns, and a booking
// service that owned them would be a booking service that has to be edited
// when marketing rewrites a sentence.
type JobNotifier struct{ service *Service }

func NewJobNotifier(service *Service) *JobNotifier { return &JobNotifier{service: service} }

// jobCopy is what a customer is told, per status.
//
// Statuses not listed here are deliberately silent. A customer does not need a
// push for ASSIGNED — the driver has not accepted yet and the assignment can
// still move — and being buzzed for every internal transition is how people
// learn to turn notifications off, which then costs them the arrival they did
// want.
var jobCopy = map[string]struct {
	title string
	body  string
}{
	"ACCEPTED":    {"A driver is on the way", "Your driver has accepted and is heading to you."},
	"ARRIVING":    {"Your driver is nearby", "Your driver is close. Please be ready."},
	"AT_PICKUP":   {"Your driver has arrived", "Your driver is waiting at the pickup point."},
	"IN_PROGRESS": {"On the way", "Your trip has started."},
	"COMPLETED":   {"Arrived", "Your trip is complete. Thanks for riding with us."},
	"CANCELLED":   {"Cancelled", "This booking was cancelled."},
	"EXPIRED":     {"No driver found", "We could not find a driver. Please try again."},
	"FAILED":      {"Something went wrong", "This booking could not be completed."},
}

// JobStatusChanged notifies the customer, when the status is one worth a buzz.
func (n *JobNotifier) JobStatusChanged(ctx context.Context, jobID, userID, jobType, status string) error {
	if n == nil || n.service == nil || userID == "" || jobID == "" {
		return nil
	}
	wording, worthTelling := jobCopy[status]
	if !worthTelling {
		return nil
	}

	_, err := n.service.Notify(ctx, Message{
		UserID:   userID,
		Category: categoryForJobType(jobType),
		Title:    wording.title,
		Body:     wording.body,
		// Document 122: a stable deep-link route into the relevant screen.
		DeepLink: "rideme://jobs/" + jobID,
		Data:     map[string]any{"job_id": jobID, "status": status, "job_type": jobType},
		// One buzz per job per status, however many times the transition is
		// retried or replayed.
		IdempotencyKey: keyFor(jobID, status),
	})
	return err
}

// keyFor is one buzz per job per status.
func keyFor(jobID, status string) string {
	return fmt.Sprintf("job:%s:%s", jobID, status)
}

// categoryForJobType maps a service onto document 122's categories, so a
// customer can mute grocery updates without muting their ride.
func categoryForJobType(jobType string) Category {
	switch strings.ToUpper(jobType) {
	case "GROCERY":
		return CategoryOrder
	case "PARCEL", "CARGO", "FREIGHT":
		return CategoryDelivery
	default:
		return CategoryRide
	}
}
