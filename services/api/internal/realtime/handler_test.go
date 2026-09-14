package realtime_test

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sarmadkung/rideme/services/api/internal/identity"
	"github.com/sarmadkung/rideme/services/api/internal/realtime"
)

// authenticateAs stands in for the identity middleware.
func authenticateAs(p identity.Principal) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(identity.ContextWithPrincipal(r.Context(), p)))
		})
	}
}

type staticDriver struct{ id string }

func (s staticDriver) DriverIDForUser(context.Context, string) (string, error) {
	return s.id, nil
}

func serverFor(t *testing.T, hub *realtime.Hub, p identity.Principal, driverID string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	realtime.NewHandler(hub, staticDriver{id: driverID}).Routes(mux, authenticateAs(p))
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func customerHub(membership func(realtime.Subscriber, realtime.Channel) (bool, error)) *realtime.Hub {
	return realtime.NewHub(realtime.RoleAuthorizer{Membership: membership})
}

// The whole point of the slice: an event published into the hub arrives at a
// client over HTTP. Before this handler existed the hub had no transport at
// all, so every event it could route had nowhere to go.
func TestASubscriberReceivesAPublishedEvent(t *testing.T) {
	hub := customerHub(func(realtime.Subscriber, realtime.Channel) (bool, error) { return true, nil })
	server := serverFor(t, hub, identity.Principal{UserID: "user-1", SessionID: "s1"}, "")

	req, err := http.NewRequest(http.MethodGet, server.URL+"/v1/realtime?channel=job:job-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		t.Fatalf("open the stream: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("content type %q, want text/event-stream", got)
	}

	reader := bufio.NewReader(resp.Body)
	// The subscription acknowledgement, written before any event so a client
	// knows it is connected.
	if _, err := reader.ReadString('\n'); err != nil {
		t.Fatalf("read the acknowledgement: %v", err)
	}

	// The handler subscribes before writing the acknowledgement, so by now the
	// connection is on the channel.
	waitForSubscriber(t, hub, realtime.Channel{Kind: realtime.ChannelJob, ID: "job-1"})
	realtime.NewPublisher(hub, nil).JobChanged(context.Background(),
		"job-1", "user-1", "driver-1", "RIDE", "IN_PROGRESS")

	frame := readFrame(t, reader)
	if !strings.Contains(frame, "event: job.status_changed") {
		t.Errorf("frame %q does not name the event type", frame)
	}
	if !strings.Contains(frame, `"status":"IN_PROGRESS"`) {
		t.Errorf("frame %q does not carry the status", frame)
	}
}

// Document 047: "Authorization is mandatory before subscription." A client
// that names somebody else's job is refused the whole stream rather than
// quietly given an empty one — silence is indistinguishable from nothing
// happening, and a client that believes it is watching is worse off than one
// told it cannot.
func TestAChannelTheSubscriberCannotHaveIsRefused(t *testing.T) {
	hub := customerHub(func(realtime.Subscriber, realtime.Channel) (bool, error) { return false, nil })
	server := serverFor(t, hub, identity.Principal{UserID: "user-2", SessionID: "s2"}, "")

	resp, err := http.Get(server.URL + "/v1/realtime?channel=job:somebody-elses-job")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status %d, want 403", resp.StatusCode)
	}
	if hub.ConnectionCount() != 0 {
		t.Errorf("%d connections left open after a refusal", hub.ConnectionCount())
	}
}

// The channel grammar is the first line of defence and the client controls the
// string.
func TestAMalformedChannelIsRefusedBeforeAnythingIsOpened(t *testing.T) {
	hub := customerHub(func(realtime.Subscriber, realtime.Channel) (bool, error) { return true, nil })
	server := serverFor(t, hub, identity.Principal{UserID: "user-3", SessionID: "s3"}, "")

	for _, channel := range []string{"", "job", "wildcard:*x:y", "secrets:everything"} {
		resp, err := http.Get(server.URL + "/v1/realtime?channel=" + channel)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			t.Errorf("channel %q was accepted", channel)
		}
	}
	if hub.ConnectionCount() != 0 {
		t.Errorf("%d connections left open", hub.ConnectionCount())
	}
}

// A subscription with no channel is a client bug that would otherwise hold an
// idle connection open against the per-account limit forever.
func TestAStreamWithNoChannelIsRefused(t *testing.T) {
	hub := customerHub(func(realtime.Subscriber, realtime.Channel) (bool, error) { return true, nil })
	server := serverFor(t, hub, identity.Principal{UserID: "user-4", SessionID: "s4"}, "")

	resp, err := http.Get(server.URL + "/v1/realtime")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Error("a stream with no channel was accepted")
	}
}

// A driver's own position reaches their own channel; the job channel is what
// the customer is allowed to watch. Publishing to only one of them would mean
// either the driver's app or the customer's map sees nothing.
func TestAPositionReachesBothTheDriverAndTheJob(t *testing.T) {
	hub := customerHub(func(realtime.Subscriber, realtime.Channel) (bool, error) { return true, nil })
	driverConn, err := hub.Connect("d", realtime.Subscriber{UserID: "u-d", DriverID: "driver-9"})
	if err != nil {
		t.Fatal(err)
	}
	if err := hub.Subscribe(driverConn, realtime.Channel{Kind: realtime.ChannelDriver, ID: "driver-9"}); err != nil {
		t.Fatal(err)
	}
	customerConn, err := hub.Connect("c", realtime.Subscriber{UserID: "u-c"})
	if err != nil {
		t.Fatal(err)
	}
	if err := hub.Subscribe(customerConn, realtime.Channel{Kind: realtime.ChannelJob, ID: "job-9"}); err != nil {
		t.Fatal(err)
	}

	realtime.NewPublisher(hub, nil).DriverMoved(context.Background(), realtime.DriverPosition{
		DriverID: "driver-9", JobID: "job-9", Latitude: 31.5204, Longitude: 74.3587,
		RecordedAt: time.Now().UTC(),
	})

	for name, conn := range map[string]*realtime.Connection{"driver": driverConn, "customer": customerConn} {
		select {
		case event := <-conn.Events():
			if event.Type != realtime.EventDriverLocation {
				t.Errorf("%s received %q, want driver.location", name, event.Type)
			}
		case <-time.After(time.Second):
			t.Errorf("%s received nothing", name)
		}
	}
}

func waitForSubscriber(t *testing.T, hub *realtime.Hub, ch realtime.Channel) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if hub.SubscriberCount(ch) > 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("nobody subscribed to %s", ch)
}

// readFrame reads lines until the blank line that ends an SSE frame, skipping
// heartbeat comments.
func readFrame(t *testing.T, reader *bufio.Reader) string {
	t.Helper()
	var frame strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read the stream: %v", err)
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.TrimSpace(line) == "" {
			if frame.Len() > 0 {
				return frame.String()
			}
			continue
		}
		frame.WriteString(line)
	}
}
