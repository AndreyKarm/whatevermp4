package rooms

import (
	"context"
	"encoding/json"
	"liotom/whateverserver/internal/events"
	"log"
	"slices"
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
		st.Queue = slices.Clone(st.Queue)
		st.Position, st.UpdatedAt = r.posLocked(nowMs()), nowMs()
		r.mu.Unlock()
		out[r.ID] = LiveRoom{Members: names, State: st}
	}
	return out
}

// changed tells the rooms list that a room's viewers or playback changed.
func (h *Hub) changed(id string) {
	h.bus.Send(events.Event{Type: "rooms", Op: "UPDATE", ID: id})
}

// mediaDurations is the rooms' mediaLookup: ready media and their lengths.
func (h *Hub) mediaDurations(ctx context.Context, ids []string) (map[string]float64, error) {
	rows, err := h.db.Query(ctx, `
		SELECT id, COALESCE(duration_sec, 0) FROM media_files
		WHERE id = ANY($1) AND status = 'ready'`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]float64, len(ids))
	for rows.Next() {
		var id string
		var d float64
		if err := rows.Scan(&id, &d); err != nil {
			return nil, err
		}
		out[id] = d
	}
	return out, rows.Err()
}

// Join loads the room (if needed) and adds the client atomically, so the
// eviction loop can't remove a room mid-join. A private room needs its
// invite code (ErrInviteRequired otherwise).
func (h *Hub) Join(ctx context.Context, id, invite string, c *Client) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	r, ok := h.rooms[id]
	if !ok {
		var a Access
		var code *string
		var raw []byte
		if err := h.db.QueryRow(ctx, `
			SELECT name, owner_name, state, everyone_controls, controllers, private,
				password_hash IS NOT NULL, invite_code
			FROM rooms WHERE id=$1`, id).
			Scan(&a.Name, &a.Owner, &raw, &a.Everyone, &a.Controllers, &a.Private, &a.HasPassword, &code); err != nil {
			return err
		}
		if code != nil {
			a.InviteCode = *code
		}
		r = newRoom(id, a)
		r.onChange = func() { h.changed(id) }
		r.lookup = h.mediaDurations
		_ = json.Unmarshal(raw, &r.state)
		if r.state.Rate <= 0 {
			r.state.Rate = 1
		}
		r.state.Playing = false // never resume "playing" after a restart
		// durations for the end timer once someone presses play
		if ids := append([]string{r.state.MediaID}, r.state.Queue...); r.state.MediaID != "" {
			if d, err := h.mediaDurations(ctx, ids); err == nil {
				r.durations = d
			}
		}
		h.rooms[id] = r
	}
	if err := r.join(c, invite); err != nil {
		return err
	}
	h.changed(id)
	return nil
}

// SetAccess applies a settings change (owner, controllers, privacy…) to a
// loaded room.
func (h *Hub) SetAccess(id string, a Access) {
	h.mu.Lock()
	r := h.rooms[id]
	h.mu.Unlock()
	if r != nil {
		r.setAccess(a)
	}
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
	r.stop()
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
		h.changed(c.room.ID)
	}
}

func (h *Hub) Run(ctx context.Context) {
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	// members with fresh sync status go out at most this often
	m := time.NewTicker(2 * time.Second)
	defer m.Stop()
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
		case <-m.C:
			for _, r := range h.loaded() {
				r.flushMembers()
			}
		}
	}
}

func (h *Hub) loaded() []*Room {
	h.mu.Lock()
	defer h.mu.Unlock()
	list := make([]*Room, 0, len(h.rooms))
	for _, r := range h.rooms {
		list = append(list, r)
	}
	return list
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
	for _, r := range h.loaded() {
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
				r.stop()
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
