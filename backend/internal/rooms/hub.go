package rooms

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Hub struct {
	db    *pgxpool.Pool
	mu    sync.Mutex
	rooms map[string]*Room
}

func NewHub(db *pgxpool.Pool) *Hub {
	return &Hub{db: db, rooms: map[string]*Room{}}
}

// Join loads the room (if needed) and adds the client atomically,
// so the eviction loop can't remove a room mid-join.
func (h *Hub) Join(ctx context.Context, id string, c *Client) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	r, ok := h.rooms[id]
	if !ok {
		var owner string
		var raw []byte
		var everyone bool
		if err := h.db.QueryRow(ctx,
			`SELECT owner_id, state, everyone_controls FROM rooms WHERE id=$1`, id).
			Scan(&owner, &raw, &everyone); err != nil {
			return err
		}
		r = newRoom(id, owner, everyone)
		_ = json.Unmarshal(raw, &r.state)
		if r.state.Rate <= 0 {
			r.state.Rate = 1
		}
		r.state.Playing = false // never resume "playing" after a restart
		h.rooms[id] = r
	}
	r.join(c)
	return nil
}

func (h *Hub) Leave(c *Client) {
	if c.room != nil {
		c.room.leave(c)
	}
}

func (h *Hub) Run(ctx context.Context) {
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			fctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			h.flush(fctx, false)
			cancel()
			return
		case <-t.C:
			h.flush(ctx, true)
		}
	}
}

func (h *Hub) flush(ctx context.Context, evict bool) {
	h.mu.Lock()
	list := make([]*Room, 0, len(h.rooms))
	for _, r := range h.rooms {
		list = append(list, r)
	}
	h.mu.Unlock()

	for _, r := range list {
		if st, dirty := r.persistSnapshot(); dirty {
			b, _ := json.Marshal(st)
			if _, err := h.db.Exec(ctx,
				`UPDATE rooms SET state=$2, updated_at=now() WHERE id=$1`, r.ID, b); err != nil {
				log.Printf("persist room %s: %v", r.ID, err)
			}
		}
		if evict {
			h.mu.Lock()
			if r.empty() {
				delete(h.rooms, r.ID)
			}
			h.mu.Unlock()
		}
	}
}
