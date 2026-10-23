package rooms

import (
	"cmp"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"math"
	"slices"
	"strings"
	"sync"
	"time"
)

// Access says who may join and control a room. The hub keeps it in sync
// with the rooms row (owner_name, everyone_controls, controllers, private,
// invite_code).
type Access struct {
	Owner       string
	Everyone    bool
	Controllers []string // nicknames the owner allowed to control playback
	Private     bool
	InviteCode  string // required to join a private room
}

// ErrInviteRequired: a private room was joined without its invite code.
var ErrInviteRequired = errors.New("invite required")

// mediaLookup returns the duration in seconds (0 if unknown) of each ID
// that is ready media; others are left out.
type mediaLookup func(ctx context.Context, ids []string) (map[string]float64, error)

const maxQueue = 200

type Room struct {
	ID string

	mu           sync.Mutex
	access       Access
	state        State
	holding      bool
	dirty        bool
	membersDirty bool // a status arrived; members go out on the hub's next tick
	clients      map[*Client]struct{}
	durations    map[string]float64 // media ID → seconds, for the end timer
	endTimer     *time.Timer
	closed       bool

	onChange func() // playback state changed (play/pause/media/rate/queue)
	lookup   mediaLookup
}

func newRoom(id string, a Access) *Room {
	return &Room{ID: id, access: a, state: State{Rate: 1},
		clients: map[*Client]struct{}{}, durations: map[string]float64{}}
}

// nowMs is the server clock; a variable so tests can move it.
var nowMs = func() int64 { return time.Now().UnixMilli() }

func (r *Room) posLocked(now int64) float64 {
	if !r.state.Playing {
		return r.state.Position
	}
	return r.state.Position + float64(now-r.state.UpdatedAt)/1000*r.state.Rate
}

// snapshotLocked folds elapsed time into Position so fields can be changed.
func (r *Room) snapshotLocked() {
	now := nowMs()
	r.state.Position = r.posLocked(now)
	r.state.UpdatedAt = now
}

func (r *Room) canControlLocked(c *Client) bool {
	a := r.access
	return a.Everyone || c.Nickname == a.Owner || slices.Contains(a.Controllers, c.Nickname)
}

func (r *Room) membersLocked() []Member {
	ms := make([]Member, 0, len(r.clients))
	for c := range r.clients {
		m := Member{Name: c.Nickname, Buffering: c.buffering}
		if c.hasStatus {
			drift, ahead := c.drift, c.ahead
			m.Drift, m.Ahead = &drift, &ahead
		}
		ms = append(ms, m)
	}
	// a stable order, so the list doesn't reshuffle on every update
	slices.SortFunc(ms, func(a, b Member) int { return cmp.Compare(a.Name, b.Name) })
	return ms
}

func (r *Room) stateMsgLocked() Out {
	st := r.state
	return Out{Type: "state", State: &st, Holding: r.holding, Server: nowMs(),
		Owner: r.access.Owner, Everyone: r.access.Everyone, Controllers: r.access.Controllers}
}

func (r *Room) broadcastLocked(o Out) {
	b, _ := json.Marshal(o)
	for c := range r.clients {
		c.trySend(b)
	}
}

// broadcastStateLocked sends the state to everyone and re-arms the end timer.
func (r *Room) broadcastStateLocked() {
	r.broadcastLocked(r.stateMsgLocked())
	r.scheduleEndLocked()
}

func (r *Room) broadcastMembersLocked() {
	r.membersDirty = false
	r.broadcastLocked(Out{Type: "members", Members: r.membersLocked()})
}

// applyBufferLocked implements "wait for everyone". Returns true if state changed.
func (r *Room) applyBufferLocked() bool {
	any := false
	for c := range r.clients {
		if c.buffering {
			any = true
			break
		}
	}
	switch {
	case any && r.state.Playing:
		r.snapshotLocked()
		r.state.Playing, r.holding = false, true
		return true
	case !any && r.holding:
		r.snapshotLocked()
		r.state.Playing, r.holding = true, false
		return true
	}
	return false
}

func (r *Room) join(c *Client, invite string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.access.Private && !validInvite(r.access.InviteCode, invite) {
		return ErrInviteRequired
	}
	c.room = r
	r.clients[c] = struct{}{}
	c.sendOut(r.stateMsgLocked())
	r.broadcastMembersLocked()
	return nil
}

func validInvite(code, given string) bool {
	return code != "" && subtle.ConstantTimeCompare([]byte(code), []byte(given)) == 1
}

func (r *Room) leave(c *Client) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.clients, c)
	if r.applyBufferLocked() {
		r.broadcastStateLocked()
	}
	r.broadcastMembersLocked()
}

// receive looks up the media a message names (outside the lock, it hits
// the database), then handles it.
func (r *Room) receive(ctx context.Context, c *Client, in In) {
	var ids []string
	switch in.Type {
	case "change_media":
		if in.MediaID != "" {
			ids = []string{in.MediaID}
		}
	case "queue":
		ids = in.Queue
		if len(ids) > maxQueue {
			ids = ids[:maxQueue]
		}
	}
	var known map[string]float64
	if len(ids) > 0 {
		if r.lookup == nil {
			known = make(map[string]float64, len(ids))
			for _, id := range ids {
				known[id] = 0
			}
		} else {
			lctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			var err error
			known, err = r.lookup(lctx, ids)
			cancel()
			if err != nil {
				c.sendOut(Out{Type: "error", Text: "couldn't look up that media"})
				return
			}
		}
	}
	r.handle(c, in, known)
}

// handle applies a client message; known holds the ready media (with
// durations) among the IDs it names.
func (r *Room) handle(c *Client, in In, known map[string]float64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	switch in.Type {
	case "buffering":
		c.buffering = in.Buffering
		if r.applyBufferLocked() {
			r.broadcastStateLocked()
		}
		r.broadcastMembersLocked()
		return
	case "status":
		if finite(in.Drift) && finite(in.Ahead) {
			c.hasStatus = true
			c.drift = math.Max(-1e5, math.Min(in.Drift, 1e5))
			c.ahead = math.Max(0, math.Min(in.Ahead, 1e5))
			r.membersDirty = true
		}
		return
	case "chat":
		t := strings.TrimSpace(in.Text)
		if t == "" {
			return
		}
		if r := []rune(t); len(r) > 500 {
			t = string(r[:500])
		}
		r.broadcastLocked(Out{Type: "chat", Text: t,
			From: &Member{Name: c.Nickname}})
		return
	}

	if !r.canControlLocked(c) {
		c.sendOut(Out{Type: "error", Text: "only the host can control playback"})
		return
	}
	for id, d := range known {
		r.durations[id] = d
	}
	pos := in.Position
	if pos < 0 {
		pos = 0
	}
	switch in.Type {
	case "play":
		r.state.Position, r.state.UpdatedAt, r.state.Playing = pos, nowMs(), true
	case "pause":
		r.snapshotLocked()
		r.state.Playing = false
	case "seek":
		r.state.Position, r.state.UpdatedAt = pos, nowMs()
	case "rate":
		if in.Rate < 0.25 || in.Rate > 2 {
			return
		}
		r.snapshotLocked()
		r.state.Rate = in.Rate
	case "change_media":
		if _, ok := known[in.MediaID]; !ok {
			c.sendOut(Out{Type: "error", Text: "that movie isn't ready"})
			return
		}
		// position: where the controller last watched this media (their localStorage)
		r.state = State{MediaID: in.MediaID, Position: pos, Rate: 1, UpdatedAt: nowMs(),
			Queue: slices.DeleteFunc(slices.Clone(r.state.Queue),
				func(id string) bool { return id == in.MediaID })}
	case "queue":
		q := make([]string, 0, len(in.Queue))
		for _, id := range in.Queue {
			if _, ok := known[id]; ok && len(q) < maxQueue && !slices.Contains(q, id) {
				q = append(q, id)
			}
		}
		// playback is untouched (and so is a buffering hold)
		r.state.Queue = q
		r.dirty = true
		r.broadcastStateLocked()
		r.changed()
		return
	case "next":
		if len(r.state.Queue) == 0 {
			return
		}
		r.advanceLocked()
	default:
		return
	}
	r.holding = false
	r.applyBufferLocked()
	r.dirty = true
	r.broadcastStateLocked()
	r.changed()
}

func (r *Room) changed() {
	if r.onChange != nil {
		r.onChange()
	}
}

func finite(f float64) bool { return !math.IsNaN(f) && !math.IsInf(f, 0) }

// advanceLocked starts the first queued media from the beginning.
func (r *Room) advanceLocked() {
	next, rest := r.state.Queue[0], slices.Clone(r.state.Queue[1:])
	r.state = State{MediaID: next, Playing: true, Rate: 1, UpdatedAt: nowMs(), Queue: rest}
}

// scheduleEndLocked arms a timer for when the playing media reaches its
// end, so the room moves on to the queue (or stops) without any client.
func (r *Room) scheduleEndLocked() {
	if r.endTimer != nil {
		r.endTimer.Stop()
		r.endTimer = nil
	}
	if r.closed || !r.state.Playing || r.state.Rate <= 0 {
		return
	}
	d := r.durations[r.state.MediaID]
	if d <= 0 {
		return
	}
	left := (d - r.posLocked(nowMs())) / r.state.Rate
	media := r.state.MediaID
	r.endTimer = time.AfterFunc(time.Duration(max(left, 0)*float64(time.Second)),
		func() { r.ended(media) })
}

// ended runs when media should have reached its end: play the next queued
// media, or pause at the end.
func (r *Room) ended(media string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || !r.state.Playing || r.state.MediaID != media {
		return
	}
	d := r.durations[media]
	if d <= 0 {
		return
	}
	now := nowMs()
	if r.posLocked(now) < d-0.5 {
		r.scheduleEndLocked() // rescheduled since (e.g. a seek back); not there yet
		return
	}
	if len(r.state.Queue) > 0 {
		r.advanceLocked()
	} else {
		r.state.Position, r.state.UpdatedAt, r.state.Playing = d, now, false
	}
	r.holding = false
	r.applyBufferLocked()
	r.dirty = true
	r.broadcastStateLocked()
	r.changed()
}

func (r *Room) setAccess(a Access) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.access = a
	r.broadcastStateLocked()
}

// flushMembers sends members if a status changed since the last send.
func (r *Room) flushMembers() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.membersDirty {
		r.broadcastMembersLocked()
	}
}

// stop disarms the room once the hub drops it.
func (r *Room) stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	if r.endTimer != nil {
		r.endTimer.Stop()
		r.endTimer = nil
	}
}

// persistSnapshot returns state to save if it changed since last call.
func (r *Room) persistSnapshot() (State, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.dirty {
		return State{}, false
	}
	r.dirty = false
	st := r.state
	st.Queue = slices.Clone(st.Queue) // marshalled after the lock is released
	st.Position = r.posLocked(nowMs())
	st.UpdatedAt = nowMs()
	return st, true
}

func (r *Room) empty() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.clients) == 0
}
