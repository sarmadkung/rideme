// Package messaging owns the NATS connection. Subjects and event contracts are
// deliberately absent from Phase 1 — they are defined with the events that need
// them (document 18), not speculatively.
package messaging

import (
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

type Conn struct {
	*nats.Conn
}

func Connect(url, name string) (*Conn, error) {
	conn, err := nats.Connect(url,
		nats.Name(name),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
		nats.Timeout(5*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("connect nats: %w", err)
	}
	return &Conn{Conn: conn}, nil
}

// Healthy reports whether the connection is usable right now. NATS reconnects
// on its own, so a reconnecting client is unhealthy but not fatal.
func (c *Conn) Healthy() bool {
	return c.Conn.IsConnected()
}

func (c *Conn) Close() { c.Conn.Close() }
