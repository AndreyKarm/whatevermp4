package rooms

import (
	"context"
	"encoding/json"
	"liotom/whateverserver/internal/events"
	"log"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Hub struct {
	db    *pgxpool.Pool
	bus   *events.Bus
	mu    sync.Mutex
	rooms map[string]*Room
}

func NewHub(db *pgxpool.Pool, bus *events.Bus) *Hub {
	return &Hub{db: db, bus: bus, rooms: map[string]*Room{}}
}

// LiveRoom is the in-memory view of a loaded room, fresher than the DB copy.
type LiveRoom struct {
	Members []string
	State   State
}

// Live returns members and current state of every loaded room.
func (h *Hub) Live() map[string]LiveRoom {
	h.mu.Lock()
	list := make([]*Room, 0, len(h.rooms))
	for _, r := range h.rooms {
		list = append(list, r)
	}
	h.mu.Unlock()

	out := make(map[string]LiveRoom, len(list))
	for _, r := range list {
		r.mu.Lock()
		names := make([]string, 0, len(r.clients))
		for c := range r.clients {
			names = append(names, c.Nickname)
		}
		st := r.state
		st.Position, st.UpdatedAt = r.posLocked(nowMs()), nowMs()
		r.mu.Unlock()
		out[r.ID] = LiveRoom{Members: names, State: st}
	}
	return out
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
			`SELECT owner_name, state, everyone_controls FROM rooms WHERE id=$1`, id).
			Scan(&owner, &raw, &everyone); err != nil {
			return err
		}
		r = newRoom(id, owner, everyone)
		r.onChange = func() { h.bus.Publish("rooms") }
		_ = json.Unmarshal(raw, &r.state)
		if r.state.Rate <= 0 {
			r.state.Rate = 1
		}
		r.state.Playing = false // never resume "playing" after a restart
		h.rooms[id] = r
	}
	r.join(c)
	h.bus.Publish("rooms")
	return nil
}

// SetEveryoneControls applies a settings change to a loaded room.
func (h *Hub) SetEveryoneControls(id string, v bool) {
	h.mu.Lock()
	r := h.rooms[id]
	h.mu.Unlock()
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.everyone = v
	r.broadcastLocked(r.stateMsgLocked())
}

// Remove unloads a deleted room and disconnects its clients.
func (h *Hub) Remove(id string) {
	h.mu.Lock()
	r := h.rooms[id]
	delete(h.rooms, id)
	h.mu.Unlock()
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.dirty = false
	for c := range r.clients {
		go c.conn.Close(websocket.StatusPolicyViolation, "room deleted")
	}
}

func (h *Hub) Leave(c *Client) {
	if c.room != nil {
		c.room.leave(c)
		h.bus.Publish("rooms")
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
			h.reap(ctx)
		}
	}
}

// emptyRoomGrace is how long a room may sit with nobody in it before it is
// deleted. It covers page reloads and the creator's way from the library
// into the room.
const emptyRoomGrace = 60 * time.Second

// reap deletes rooms nobody is watching: rooms that aren't loaded (an empty
// room is evicted within 10s and its updated_at touched) and haven't been
// touched for emptyRoomGrace. h.mu is held so no Join can load a room while
// it is being deleted.
func (h *Hub) reap(ctx context.Context) {
	h.mu.Lock()
	defer h.mu.Unlock()
	loaded := make([]string, 0, len(h.rooms))
	for id := range h.rooms {
		loaded = append(loaded, id)
	}
	if _, err := h.db.Exec(ctx, `
		DELETE FROM rooms
		WHERE NOT (id = ANY($1)) AND updated_at < now() - make_interval(secs => $2)`,
		loaded, emptyRoomGrace.Seconds()); err != nil {
		log.Printf("reap rooms: %v", err)
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
				// start the empty-room grace period (see reap)
				if _, err := h.db.Exec(ctx,
					`UPDATE rooms SET updated_at=now() WHERE id=$1`, r.ID); err != nil {
					log.Printf("touch room %s: %v", r.ID, err)
				}
			}
			h.mu.Unlock()
		}
	}
}
