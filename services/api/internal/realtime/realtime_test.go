package realtime_test

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/sarmadkung/rideme/services/api/internal/realtime"
)

func customer(userID string) realtime.Subscriber {
	return realtime.Subscriber{UserID: userID}
}

func admin() realtime.Subscriber {
	return realtime.Subscriber{UserID: "admin-1", Roles: []string{"ADMIN"}}
}

func openHub(t *testing.T) *realtime.Hub {
	t.Helper()
	return realtime.NewHub(realtime.RoleAuthorizer{
		Membership: func(sub realtime.Subscriber, ch realtime.Channel) (bool, error) {
			// Stands in for the database: user-1 owns job-1.
			return ch.Kind == realtime.ChannelJob && ch.ID == "job-1" && sub.UserID == "user-1", nil
		},
	})
}

func TestChannelParsingIsStrict(t *testing.T) {
	// The client controls this string. Document 47 forbids arbitrary channel
	// subscription, and refusing to construct an undefined channel is the
	// first line of that defence.
	for _, raw := range []string{"user:1", "driver:2", "job:3", "merchant:4", "admin:operations"} {
		if _, err := realtime.ParseChannel(raw); err != nil {
			t.Errorf("%q should parse: %v", raw, err)
		}
	}
	for _, raw := range []string{"", "user", "user:", ":1", "root:1", "*", "user:1:2", "admin:everything"} {
		if _, err := realtime.ParseChannel(raw); !errors.Is(err, realtime.ErrBadChannel) {
			t.Errorf("%q should be refused, got %v", raw, err)
		}
	}
}

func TestASubscriberCannotJoinSomeoneElsesChannel(t *testing.T) {
	hub := openHub(t)
	conn, err := hub.Connect("c1", customer("user-1"))
	if err != nil {
		t.Fatal(err)
	}

	if err := hub.Subscribe(conn, realtime.Channel{Kind: realtime.ChannelUser, ID: "user-1"}); err != nil {
		t.Fatalf("own channel refused: %v", err)
	}
	// The whole point: subscribing to another user's channel would leak their
	// jobs, their payments and their driver's position.
	if err := hub.Subscribe(conn, realtime.Channel{Kind: realtime.ChannelUser, ID: "user-2"}); !errors.Is(err, realtime.ErrNotAuthorized) {
		t.Fatalf("another user's channel was joined: %v", err)
	}
	if err := hub.Subscribe(conn, realtime.Channel{Kind: realtime.ChannelDriver, ID: "driver-9"}); !errors.Is(err, realtime.ErrNotAuthorized) {
		t.Fatalf("an arbitrary driver's channel was joined: %v", err)
	}
	if err := hub.Subscribe(conn, realtime.Channel{Kind: realtime.ChannelAdminOps}); !errors.Is(err, realtime.ErrNotAuthorized) {
		t.Fatalf("a customer joined admin operations: %v", err)
	}
}

func TestJobChannelRequiresMembership(t *testing.T) {
	hub := openHub(t)
	owner, _ := hub.Connect("c1", customer("user-1"))
	stranger, _ := hub.Connect("c2", customer("user-2"))

	job := realtime.Channel{Kind: realtime.ChannelJob, ID: "job-1"}
	if err := hub.Subscribe(owner, job); err != nil {
		t.Fatalf("the job's owner was refused: %v", err)
	}
	if err := hub.Subscribe(stranger, job); !errors.Is(err, realtime.ErrNotAuthorized) {
		t.Fatalf("a stranger joined a job channel: %v", err)
	}
}

func TestAuthorizationFailsClosedWithoutAMembershipCheck(t *testing.T) {
	// A gateway that fails open leaks every driver's position. With no
	// membership function configured, job and merchant channels must deny.
	hub := realtime.NewHub(realtime.RoleAuthorizer{})
	conn, _ := hub.Connect("c1", customer("user-1"))

	for _, ch := range []realtime.Channel{
		{Kind: realtime.ChannelJob, ID: "job-1"},
		{Kind: realtime.ChannelMerchant, ID: "m-1"},
	} {
		if err := hub.Subscribe(conn, ch); !errors.Is(err, realtime.ErrNotAuthorized) {
			t.Errorf("%s was allowed with no membership check: %v", ch, err)
		}
	}
}

func TestOperationsMayWatchADriver(t *testing.T) {
	hub := openHub(t)
	conn, _ := hub.Connect("c1", admin())
	if err := hub.Subscribe(conn, realtime.Channel{Kind: realtime.ChannelDriver, ID: "driver-7"}); err != nil {
		t.Fatalf("operations were refused a driver channel: %v", err)
	}
	if err := hub.Subscribe(conn, realtime.Channel{Kind: realtime.ChannelAdminOps}); err != nil {
		t.Fatalf("operations were refused the ops channel: %v", err)
	}
}

func TestPublishReachesOnlySubscribers(t *testing.T) {
	hub := openHub(t)
	subscribed, _ := hub.Connect("c1", customer("user-1"))
	other, _ := hub.Connect("c2", customer("user-2"))

	ch := realtime.Channel{Kind: realtime.ChannelUser, ID: "user-1"}
	if err := hub.Subscribe(subscribed, ch); err != nil {
		t.Fatal(err)
	}
	_ = hub.Subscribe(other, realtime.Channel{Kind: realtime.ChannelUser, ID: "user-2"})

	delivered := hub.Publish(ch, realtime.Envelope{
		EventID: "e1", Type: realtime.EventJobAssigned, ResourceID: "job-1",
	})
	if delivered != 1 {
		t.Fatalf("delivered to %d connections, want 1", delivered)
	}

	select {
	case event := <-subscribed.Events():
		if event.Type != realtime.EventJobAssigned {
			t.Fatalf("wrong event: %+v", event)
		}
		// Document 47's envelope must be complete on the wire.
		if event.Version == 0 || event.OccurredAt.IsZero() {
			t.Fatalf("envelope was incomplete: %+v", event)
		}
	default:
		t.Fatal("the subscriber received nothing")
	}

	select {
	case event := <-other.Events():
		t.Fatalf("a non-subscriber received %+v", event)
	default:
	}
}

func TestASlowClientCannotBlockThePublisher(t *testing.T) {
	// Document 47: "Slow clients must not block the system." The failure this
	// prevents is one stalled phone freezing dispatch for everyone.
	hub := openHub(t)
	conn, _ := hub.Connect("c1", customer("user-1"))
	ch := realtime.Channel{Kind: realtime.ChannelUser, ID: "user-1"}
	if err := hub.Subscribe(conn, ch); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 1000; i++ {
			hub.Publish(ch, realtime.Envelope{
				EventID: fmt.Sprintf("e%d", i), Type: realtime.EventJobStatusChanged,
				ResourceID: "job-1",
			})
		}
	}()

	select {
	case <-done:
	case <-make(chan struct{}):
		t.Fatal("publishing blocked on a slow client")
	}
	if conn.Dropped() == 0 {
		t.Fatal("nothing was dropped; the buffer is not bounded")
	}
}

func TestLocationEventsCoalesceRatherThanQueue(t *testing.T) {
	// A backlog of stale positions is worthless — the client wants where the
	// driver is, not where they were. Document 47 requires coalescing.
	hub := openHub(t)
	conn, _ := hub.Connect("c1", customer("user-1"))
	ch := realtime.Channel{Kind: realtime.ChannelUser, ID: "user-1"}
	if err := hub.Subscribe(conn, ch); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 500; i++ {
		hub.Publish(ch, realtime.Envelope{
			EventID: fmt.Sprintf("loc%d", i), Type: realtime.EventDriverLocation,
			ResourceID: "driver-1", Payload: i,
		})
	}
	// Location overflow is coalesced, not counted as dropped.
	if conn.Dropped() != 0 {
		t.Fatalf("%d location events were dropped instead of coalesced", conn.Dropped())
	}

	// Drain, then flush: the client receives the newest position, once.
	for len(conn.Events()) > 0 {
		<-conn.Events()
	}
	conn.Flush()

	var flushed []realtime.Envelope
	for len(conn.Events()) > 0 {
		flushed = append(flushed, <-conn.Events())
	}
	if len(flushed) != 1 {
		t.Fatalf("flushed %d events, want 1 coalesced position", len(flushed))
	}
	if flushed[0].Payload.(int) != 499 {
		t.Fatalf("flushed the position from iteration %v, want the newest", flushed[0].Payload)
	}
}

func TestConnectionLimitPerUser(t *testing.T) {
	// Without a limit one account can exhaust the gateway (document 47).
	hub := openHub(t)
	for i := 0; i < realtime.MaxConnectionsPerUser; i++ {
		if _, err := hub.Connect(fmt.Sprintf("c%d", i), customer("user-1")); err != nil {
			t.Fatalf("connection %d refused: %v", i, err)
		}
	}
	if _, err := hub.Connect("one-too-many", customer("user-1")); !errors.Is(err, realtime.ErrTooManyClients) {
		t.Fatalf("want ErrTooManyClients, got %v", err)
	}
	// A different user is unaffected.
	if _, err := hub.Connect("other", customer("user-2")); err != nil {
		t.Fatalf("a second user was blocked by the first: %v", err)
	}
}

func TestDisconnectReleasesSubscriptionsAndQuota(t *testing.T) {
	hub := openHub(t)
	ch := realtime.Channel{Kind: realtime.ChannelUser, ID: "user-1"}

	conn, _ := hub.Connect("c1", customer("user-1"))
	if err := hub.Subscribe(conn, ch); err != nil {
		t.Fatal(err)
	}
	if hub.SubscriberCount(ch) != 1 {
		t.Fatal("subscription was not registered")
	}

	hub.Disconnect(conn)
	if hub.SubscriberCount(ch) != 0 {
		t.Fatal("a disconnected client is still subscribed")
	}
	if hub.ConnectionCount() != 0 {
		t.Fatal("the connection was not removed")
	}
	// Publishing to a dead channel must not panic on the closed channel.
	if delivered := hub.Publish(ch, realtime.Envelope{EventID: "e1"}); delivered != 0 {
		t.Fatalf("delivered to %d dead connections", delivered)
	}
	// Quota is released, so a reconnecting client is not locked out.
	for i := 0; i < realtime.MaxConnectionsPerUser; i++ {
		if _, err := hub.Connect(fmt.Sprintf("r%d", i), customer("user-1")); err != nil {
			t.Fatalf("reconnect %d refused: %v", i, err)
		}
	}
}

func TestDisconnectIsIdempotent(t *testing.T) {
	hub := openHub(t)
	conn, _ := hub.Connect("c1", customer("user-1"))
	hub.Disconnect(conn)
	hub.Disconnect(conn) // must not panic closing an already-closed channel
}

func TestConcurrentPublishAndDisconnectAreSafe(t *testing.T) {
	// A client dropping mid-broadcast is the normal case on mobile networks.
	hub := openHub(t)
	ch := realtime.Channel{Kind: realtime.ChannelUser, ID: "user-1"}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		conn, err := hub.Connect(fmt.Sprintf("c%d", i), customer("user-1"))
		if err != nil {
			t.Fatal(err)
		}
		if err := hub.Subscribe(conn, ch); err != nil {
			t.Fatal(err)
		}
		wg.Add(1)
		go func(c *realtime.Connection) {
			defer wg.Done()
			hub.Disconnect(c)
		}(conn)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			hub.Publish(ch, realtime.Envelope{EventID: fmt.Sprintf("e%d", i), Type: realtime.EventDriverLocation})
		}
	}()
	wg.Wait()

	if hub.ConnectionCount() != 0 {
		t.Fatalf("%d connections leaked", hub.ConnectionCount())
	}
}
