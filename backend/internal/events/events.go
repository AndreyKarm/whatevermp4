// Package events pushes change notifications to browsers so they can update
// instead of polling. Each one is a JSON object whose "type" says what
// changed: a table ("media_files", "torrents", "rooms", "folders"), usually
// with "op" and "id", or a progress update ("media_progress",
// "torrent_progress") carrying the new values. The database triggers
// (migrations/0004) build most of them; the room hub sends "rooms" itself.
package events

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
)

// Event is a change the Go code announces (the triggers send the same shape).
type Event struct {
	Type string `json:"type"`
	Op   string `json:"op,omitempty"`
	ID   string `json:"id,omitempty"`
}

type Bus struct {
	mu   sync.Mutex
	subs map[chan string]struct{}
}

func NewBus() *Bus {
	return &Bus{subs: map[chan string]struct{}{}}
}

// Subscribe returns a channel of JSON payloads. The bus closes it if the
// subscriber falls behind, since a client patching its lists from events
// must not silently miss one; it should reconnect and resync.
func (b *Bus) Subscribe() (<-chan string, func()) {
	ch := make(chan string, 64)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		delete(b.subs, ch)
		b.mu.Unlock()
	}
}

// Send publishes an event.
func (b *Bus) Send(e Event) {
	p, _ := json.Marshal(e)
	b.publish(string(p))
}

// publish never blocks: a subscriber whose buffer is full is dropped (its
// channel closed), and the browser resyncs when its socket reconnects.
func (b *Bus) publish(payload string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- payload:
		default:
			delete(b.subs, ch)
			close(ch)
		}
	}
}

// Listen relays Postgres NOTIFY on the "events" channel (sent by table
// triggers, see migrations/0004) to the bus. It uses its own connection
// rather than the pool and reconnects until ctx is done.
func Listen(ctx context.Context, dsn string, b *Bus) {
	for ctx.Err() == nil {
		if err := listenOnce(ctx, dsn, b); err != nil && ctx.Err() == nil {
			log.Printf("events listen: %v", err)
			select {
			case <-ctx.Done():
			case <-time.After(3 * time.Second):
			}
		}
	}
}

func listenOnce(ctx context.Context, dsn string, b *Bus) error {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return err
	}
	defer conn.Close(context.Background())
	if _, err := conn.Exec(ctx, "LISTEN events"); err != nil {
		return err
	}
	for {
		n, err := conn.WaitForNotification(ctx)
		if err != nil {
			return err
		}
		if json.Valid([]byte(n.Payload)) {
			b.publish(n.Payload)
		}
	}
}
