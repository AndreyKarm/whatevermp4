package rooms

import (
	"context"
	"errors"
	"liotom/whateverserver/internal/events"
	"liotom/whateverserver/internal/testdb"
	"testing"
)

func TestHubJoinLoadsRoomAndChecksInvite(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	h := NewHub(db, events.NewBus())
	_, err := db.Exec(ctx, `
		INSERT INTO media_files (id, uploaded_by, filename, status, raw_key, duration_sec)
		VALUES ('m1', 'ann', 'Ep 1', 'ready', 'raw/m1/a.mkv', 1200);
		INSERT INTO rooms (id, owner_name, state, private, invite_code, controllers)
		VALUES ('open', 'ann', '{"mediaId":"m1","playing":true,"position":30,"updatedAt":1,"rate":1}', false, NULL, '["bob"]'),
		       ('closed', 'ann', '{}', true, 'code', '[]')`)
	if err != nil {
		t.Fatal(err)
	}

	c := &Client{Nickname: "bob", send: make(chan []byte, 64)}
	if err := h.Join(ctx, "open", "", c); err != nil {
		t.Fatal(err)
	}
	r := c.room
	r.mu.Lock()
	playing, pos, d := r.state.Playing, r.state.Position, r.durations["m1"]
	canControl := r.canControlLocked(c)
	r.mu.Unlock()
	if playing || pos != 30 {
		t.Fatalf("a loaded room must start paused where it was: playing=%v pos=%v", playing, pos)
	}
	if d != 1200 {
		t.Fatalf("duration for the end timer: %v", d)
	}
	if !canControl {
		t.Fatal("controllers from the row weren't loaded")
	}

	err = h.Join(ctx, "closed", "", &Client{Nickname: "eve", send: make(chan []byte, 8)})
	if !errors.Is(err, ErrInviteRequired) {
		t.Fatalf("private room without invite: %v", err)
	}
	if err := h.Join(ctx, "closed", "code", &Client{Nickname: "eve", send: make(chan []byte, 8)}); err != nil {
		t.Fatalf("private room with invite: %v", err)
	}
	if err := h.Join(ctx, "nope", "", &Client{Nickname: "x", send: make(chan []byte, 8)}); err == nil {
		t.Fatal("joined a room that doesn't exist")
	}

	live := h.Live()
	if len(live["open"].Members) != 1 || live["open"].State.MediaID != "m1" {
		t.Fatalf("live: %+v", live)
	}
}

func TestHubEvictsAndReapsEmptyRooms(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	h := NewHub(db, events.NewBus())
	_, err := db.Exec(ctx, `
		INSERT INTO rooms (id, owner_name, updated_at) VALUES
			('stale', 'ann', now() - interval '5 minutes'),
			('fresh', 'ann', now()),
			('busy', 'ann', now() - interval '5 minutes')`)
	if err != nil {
		t.Fatal(err)
	}
	c := &Client{Nickname: "bob", send: make(chan []byte, 64)}
	if err := h.Join(ctx, "busy", "", c); err != nil {
		t.Fatal(err)
	}

	h.reap(ctx)
	var ids []string
	rows, _ := db.Query(ctx, `SELECT id FROM rooms ORDER BY id`)
	for rows.Next() {
		var id string
		_ = rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	if len(ids) != 2 || ids[0] != "busy" || ids[1] != "fresh" {
		t.Fatalf("after reap: %v (stale should be gone, the loaded room kept)", ids)
	}

	// once its last viewer leaves, the room is evicted and its grace period starts
	h.Leave(c)
	h.flush(ctx, true)
	if _, loaded := h.Live()["busy"]; loaded {
		t.Fatal("empty room still loaded")
	}
	var touched bool
	_ = db.QueryRow(ctx, `SELECT updated_at > now() - interval '1 minute' FROM rooms WHERE id='busy'`).Scan(&touched)
	if !touched {
		t.Fatal("evicting should touch updated_at")
	}
}
