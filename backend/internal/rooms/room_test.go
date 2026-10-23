package rooms

import (
	"context"
	"encoding/json"
	"math"
	"testing"
)

// fakeClock pins nowMs for the test.
func fakeClock(t *testing.T, start int64) *int64 {
	t.Helper()
	now := start
	prev := nowMs
	nowMs = func() int64 { return now }
	t.Cleanup(func() { nowMs = prev })
	return &now
}

func newTestRoom(t *testing.T, a Access, durations map[string]float64) *Room {
	t.Helper()
	r := newRoom("room", a)
	r.lookup = func(_ context.Context, ids []string) (map[string]float64, error) {
		out := map[string]float64{}
		for _, id := range ids {
			if d, ok := durations[id]; ok {
				out[id] = d
			}
		}
		return out, nil
	}
	t.Cleanup(r.stop)
	return r
}

func client(t *testing.T, r *Room, name string) *Client {
	t.Helper()
	c := &Client{Nickname: name, send: make(chan []byte, 512)}
	if err := r.join(c, r.access.InviteCode); err != nil {
		t.Fatal(err)
	}
	return c
}

// drain returns the messages the client got since the last call.
func drain(c *Client) []Out {
	var out []Out
	for {
		select {
		case b := <-c.send:
			var o Out
			_ = json.Unmarshal(b, &o)
			out = append(out, o)
		default:
			return out
		}
	}
}

func lastOf(msgs []Out, typ string) *Out {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Type == typ {
			return &msgs[i]
		}
	}
	return nil
}

func send(r *Room, c *Client, in In) { r.receive(context.Background(), c, in) }

func TestPlaybackPermissions(t *testing.T) {
	fakeClock(t, 1_000_000)
	r := newTestRoom(t, Access{Owner: "host"}, nil)
	host, guest, helper := client(t, r, "host"), client(t, r, "guest"), client(t, r, "helper")

	send(r, guest, In{Type: "play", Position: 5})
	if r.state.Playing {
		t.Fatal("a guest started playback")
	}
	if e := lastOf(drain(guest), "error"); e == nil {
		t.Fatal("guest got no error")
	}

	send(r, host, In{Type: "play", Position: 5})
	if !r.state.Playing || r.state.Position != 5 {
		t.Fatalf("host play: %+v", r.state)
	}

	// the owner hands control to one person, not everyone
	r.setAccess(Access{Owner: "host", Controllers: []string{"helper"}})
	send(r, helper, In{Type: "pause"})
	if r.state.Playing {
		t.Fatal("a controller couldn't pause")
	}
	send(r, guest, In{Type: "play"})
	if r.state.Playing {
		t.Fatal("a guest could play after controllers were set")
	}
	if st := lastOf(drain(host), "state"); st == nil || len(st.Controllers) != 1 {
		t.Fatalf("state should list controllers: %+v", st)
	}

	// ownership moves: the old owner loses control
	r.setAccess(Access{Owner: "guest"})
	send(r, host, In{Type: "play"})
	if r.state.Playing {
		t.Fatal("the previous owner still controls playback")
	}
	send(r, guest, In{Type: "play"})
	if !r.state.Playing {
		t.Fatal("the new owner can't play")
	}

	r.setAccess(Access{Owner: "guest", Everyone: true})
	send(r, host, In{Type: "pause"})
	if r.state.Playing {
		t.Fatal("everyone_controls didn't let others pause")
	}
}

func TestClockAndSeekRate(t *testing.T) {
	now := fakeClock(t, 1_000_000)
	r := newTestRoom(t, Access{Owner: "host"}, nil)
	host := client(t, r, "host")

	send(r, host, In{Type: "play", Position: 10})
	*now += 4000
	if got := r.posLocked(*now); math.Abs(got-14) > 1e-9 {
		t.Fatalf("position after 4s: %v", got)
	}
	send(r, host, In{Type: "rate", Rate: 2})
	*now += 1000
	if got := r.posLocked(*now); math.Abs(got-16) > 1e-9 {
		t.Fatalf("position at 2x: %v", got)
	}
	send(r, host, In{Type: "rate", Rate: 5}) // out of range: ignored
	if r.state.Rate != 2 {
		t.Fatalf("rate %v", r.state.Rate)
	}
	send(r, host, In{Type: "seek", Position: -3})
	if r.state.Position != 0 {
		t.Fatalf("negative seek not clamped: %v", r.state.Position)
	}
}

func TestBufferingHoldsTheRoom(t *testing.T) {
	now := fakeClock(t, 1_000_000)
	r := newTestRoom(t, Access{Owner: "host"}, nil)
	host, guest := client(t, r, "host"), client(t, r, "guest")
	send(r, host, In{Type: "play", Position: 0})

	*now += 2000
	send(r, guest, In{Type: "buffering", Buffering: true})
	if r.state.Playing || !r.holding || r.state.Position != 2 {
		t.Fatalf("should hold at 2s: %+v holding=%v", r.state, r.holding)
	}
	// editing the queue while held doesn't release the hold
	send(r, host, In{Type: "queue", Queue: nil})
	*now += 5000
	send(r, guest, In{Type: "buffering", Buffering: false})
	if !r.state.Playing || r.holding || r.state.Position != 2 {
		t.Fatalf("should resume at 2s: %+v holding=%v", r.state, r.holding)
	}
	m := lastOf(drain(host), "members")
	if m == nil || len(m.Members) != 2 {
		t.Fatalf("members: %+v", m)
	}
}

func TestLeavingReleasesHold(t *testing.T) {
	fakeClock(t, 1_000_000)
	r := newTestRoom(t, Access{Owner: "host"}, nil)
	host, guest := client(t, r, "host"), client(t, r, "guest")
	send(r, host, In{Type: "play"})
	send(r, guest, In{Type: "buffering", Buffering: true})
	r.leave(guest)
	if !r.state.Playing {
		t.Fatal("room still held after the buffering member left")
	}
}

func TestQueueAndAutoAdvance(t *testing.T) {
	now := fakeClock(t, 1_000_000)
	r := newTestRoom(t, Access{Owner: "host"}, map[string]float64{"e1": 100, "e2": 200, "e3": 300})
	host, guest := client(t, r, "host"), client(t, r, "guest")

	send(r, host, In{Type: "change_media", MediaID: "e1"})
	send(r, host, In{Type: "queue", Queue: []string{"e2", "missing", "e3", "e2"}})
	if got := r.state.Queue; len(got) != 2 || got[0] != "e2" || got[1] != "e3" {
		t.Fatalf("queue should drop unknown and duplicate IDs: %v", got)
	}
	send(r, guest, In{Type: "queue", Queue: []string{"e3"}})
	if len(r.state.Queue) != 2 {
		t.Fatal("a guest edited the queue")
	}

	send(r, host, In{Type: "play", Position: 90})
	if r.endTimer == nil {
		t.Fatal("no end timer while playing media with a known length")
	}
	// fired too early (e.g. after a seek back): nothing happens
	*now += 5000
	r.ended("e1")
	if r.state.MediaID != "e1" {
		t.Fatalf("advanced before the end: %+v", r.state)
	}
	*now += 5000 // now at 100s
	r.ended("e1")
	if r.state.MediaID != "e2" || !r.state.Playing || r.state.Position != 0 {
		t.Fatalf("didn't move on to e2: %+v", r.state)
	}
	if len(r.state.Queue) != 1 || r.state.Queue[0] != "e3" {
		t.Fatalf("queue after advance: %v", r.state.Queue)
	}
	if st := lastOf(drain(guest), "state"); st == nil || st.State.MediaID != "e2" {
		t.Fatalf("guest wasn't told: %+v", st)
	}

	// a stale timer for the previous media does nothing
	r.ended("e1")
	if r.state.MediaID != "e2" {
		t.Fatal("stale end timer changed media")
	}

	send(r, host, In{Type: "next"})
	if r.state.MediaID != "e3" || len(r.state.Queue) != 0 {
		t.Fatalf("next: %+v", r.state)
	}

	// end of the last item: pause at the end instead of running on
	*now += 300_000
	r.ended("e3")
	if r.state.Playing || r.state.Position != 300 {
		t.Fatalf("should stop at the end: %+v", r.state)
	}
}

func TestChangeMediaValidatesAndDequeues(t *testing.T) {
	fakeClock(t, 1_000_000)
	r := newTestRoom(t, Access{Owner: "host"}, map[string]float64{"a": 10, "b": 10})
	host := client(t, r, "host")
	send(r, host, In{Type: "change_media", MediaID: "nope"})
	if r.state.MediaID != "" {
		t.Fatal("switched to media that isn't ready")
	}
	send(r, host, In{Type: "queue", Queue: []string{"a", "b"}})
	send(r, host, In{Type: "change_media", MediaID: "b", Position: 3})
	if r.state.MediaID != "b" || r.state.Position != 3 {
		t.Fatalf("%+v", r.state)
	}
	if len(r.state.Queue) != 1 || r.state.Queue[0] != "a" {
		t.Fatalf("playing media should leave the queue: %v", r.state.Queue)
	}
}

func TestStatusIsThrottled(t *testing.T) {
	fakeClock(t, 1_000_000)
	r := newTestRoom(t, Access{Owner: "host"}, nil)
	host, guest := client(t, r, "host"), client(t, r, "guest")
	drain(host)

	send(r, guest, In{Type: "status", Drift: -1.5, Ahead: 12})
	send(r, guest, In{Type: "status", Drift: math.NaN(), Ahead: 1}) // ignored
	if msgs := drain(host); len(msgs) != 0 {
		t.Fatalf("status should wait for the members tick, got %+v", msgs)
	}
	r.flushMembers()
	m := lastOf(drain(host), "members")
	if m == nil {
		t.Fatal("no members after flush")
	}
	for _, mem := range m.Members {
		if mem.Name == "guest" {
			if mem.Drift == nil || *mem.Drift != -1.5 || *mem.Ahead != 12 {
				t.Fatalf("guest status: %+v", mem)
			}
			return
		}
	}
	t.Fatal("guest missing from members")
}

func TestPingIsReportedWithMembers(t *testing.T) {
	r := newTestRoom(t, Access{Owner: "host"}, nil)
	host, guest := client(t, r, "host"), client(t, r, "guest")
	r.flushMembers()
	drain(host)

	pingOf := func() *int {
		r.flushMembers()
		m := lastOf(drain(host), "members")
		if m == nil {
			return nil
		}
		for _, mem := range m.Members {
			if mem.Name == "guest" {
				return mem.Ping
			}
		}
		return nil
	}
	r.setPing(guest, 41.6)
	if p := pingOf(); p == nil || *p != 42 {
		t.Fatalf("ping: %v", p)
	}
	r.setPing(guest, 44) // under 5 ms of change: no update
	if p := pingOf(); p != nil {
		t.Fatalf("a tiny change shouldn't resend members: %v", *p)
	}
	r.setPing(guest, math.Inf(1))
	r.setPing(guest, 120)
	if p := pingOf(); p == nil || *p != 120 {
		t.Fatalf("ping after change: %v", p)
	}
}

func TestPrivateRoomNeedsInvite(t *testing.T) {
	r := newTestRoom(t, Access{Owner: "host", Private: true, InviteCode: "secret"}, nil)
	if err := r.join(&Client{Nickname: "x", send: make(chan []byte, 8)}, ""); err != ErrInviteRequired {
		t.Fatalf("join without invite: %v", err)
	}
	if err := r.join(&Client{Nickname: "x", send: make(chan []byte, 8)}, "wrong"); err != ErrInviteRequired {
		t.Fatalf("join with a wrong invite: %v", err)
	}
	if err := r.join(&Client{Nickname: "x", send: make(chan []byte, 8)}, "secret"); err != nil {
		t.Fatalf("join with the invite: %v", err)
	}
}

func TestChatTrimmedAndLimited(t *testing.T) {
	r := newTestRoom(t, Access{Owner: "host"}, nil)
	a, b := client(t, r, "a"), client(t, r, "b")
	drain(b)
	send(r, a, In{Type: "chat", Text: "   "})
	long := make([]rune, 600)
	for i := range long {
		long[i] = 'é'
	}
	send(r, a, In{Type: "chat", Text: string(long)})
	msgs := drain(b)
	if len(msgs) != 1 || len([]rune(msgs[0].Text)) != 500 || msgs[0].From.Name != "a" {
		t.Fatalf("chat: %+v", msgs)
	}
}

func TestCleanNickname(t *testing.T) {
	if got := CleanNickname("  "); got != "Anonymous" {
		t.Fatalf("empty: %q", got)
	}
	long := "ääääääääääääääääääääääääääääääääääää" // 36 runes
	if got := CleanNickname(long); len([]rune(got)) != maxNickname {
		t.Fatalf("not cut to %d runes: %q", maxNickname, got)
	}
}
