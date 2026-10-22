package rooms

import (
	"encoding/json"
	"strings"
	"sync"
	"time"
)

type Room struct {
	ID       string
	Owner    string // owner's nickname
	everyone bool

	mu      sync.Mutex
	state   State
	holding bool
	dirty   bool
	clients map[*Client]struct{}

	onChange func() // playback state changed (play/pause/media/rate)
}

func newRoom(id, owner string, everyone bool) *Room {
	return &Room{ID: id, Owner: owner, everyone: everyone,
		state: State{Rate: 1}, clients: map[*Client]struct{}{}}
}

func nowMs() int64 { return time.Now().UnixMilli() }

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

func (r *Room) canControl(c *Client) bool {
	return r.everyone || c.Nickname == r.Owner
}

func (r *Room) membersLocked() []Member {
	ms := make([]Member, 0, len(r.clients))
	for c := range r.clients {
		ms = append(ms, Member{Name: c.Nickname, Buffering: c.buffering})
	}
	return ms
}

func (r *Room) stateMsgLocked() Out {
	st := r.state
	return Out{Type: "state", State: &st, Holding: r.holding, Server: nowMs(),
		Owner: r.Owner, Everyone: r.everyone}
}

func (r *Room) broadcastLocked(o Out) {
	b, _ := json.Marshal(o)
	for c := range r.clients {
		c.trySend(b)
	}
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

func (r *Room) join(c *Client) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c.room = r
	r.clients[c] = struct{}{}
	c.sendOut(r.stateMsgLocked())
	r.broadcastLocked(Out{Type: "members", Members: r.membersLocked()})
}

func (r *Room) leave(c *Client) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.clients, c)
	if r.applyBufferLocked() {
		r.broadcastLocked(r.stateMsgLocked())
	}
	r.broadcastLocked(Out{Type: "members", Members: r.membersLocked()})
}

func (r *Room) handle(c *Client, in In) {
	r.mu.Lock()
	defer r.mu.Unlock()

	switch in.Type {
	case "buffering":
		c.buffering = in.Buffering
		if r.applyBufferLocked() {
			r.broadcastLocked(r.stateMsgLocked())
		}
		r.broadcastLocked(Out{Type: "members", Members: r.membersLocked()})
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

	if !r.canControl(c) {
		c.sendOut(Out{Type: "error", Text: "only the host can control playback"})
		return
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
		if in.MediaID == "" {
			return
		}
		// position: where the controller last watched this media (their localStorage)
		r.state = State{MediaID: in.MediaID, Position: pos, Rate: 1, UpdatedAt: nowMs()}
	default:
		return
	}
	r.holding = false
	r.applyBufferLocked()
	r.dirty = true
	r.broadcastLocked(r.stateMsgLocked())
	if r.onChange != nil {
		r.onChange()
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
	st.Position = r.posLocked(nowMs())
	st.UpdatedAt = nowMs()
	return st, true
}

func (r *Room) empty() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.clients) == 0
}
