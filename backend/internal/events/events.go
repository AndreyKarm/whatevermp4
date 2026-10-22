// Package events pushes "something changed" notifications to browsers so
// they can refetch instead of polling. Topics are table names
// ("media_files", "torrents", "rooms").
package events

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
)

type Bus struct {
	mu   sync.Mutex
	subs map[chan string]struct{}
}

func NewBus() *Bus {
	return &Bus{subs: map[chan string]struct{}{}}
}

func (b *Bus) Subscribe() (<-chan string, func()) {
	ch := make(chan string, 16)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		delete(b.subs, ch)
		b.mu.Unlock()
	}
}

// Publish never blocks: a subscriber that falls behind misses events, which
// is fine because every event just means "refetch".
func (b *Bus) Publish(topic string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- topic:
		default:
		}
	}
}

// Listen relays Postgres NOTIFY on the "events" channel (sent by table
// triggers, see migrations/0001) to the bus. It uses its own connection
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
		b.Publish(n.Payload)
	}
}
