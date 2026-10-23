package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"liotom/whateverserver/internal/config"
	"liotom/whateverserver/internal/events"
	"liotom/whateverserver/internal/rooms"
	"liotom/whateverserver/internal/testdb"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

type api struct {
	t   *testing.T
	db  *pgxpool.Pool
	srv *httptest.Server
}

// newAPI serves the router against a fresh database. Storage is nil, so
// only endpoints that don't touch S3 can be called.
func newAPI(t *testing.T) *api {
	t.Helper()
	db := testdb.New(t)
	bus := events.NewBus()
	h := NewRouter(db, nil, rooms.NewHub(db, bus), bus, config.Config{PlaybackSecret: "test"})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &api{t: t, db: db, srv: srv}
}

// do sends a request as nickname `as` and decodes a JSON reply into out.
func (a *api) do(as, method, path string, body any, out any, headers ...string) int {
	a.t.Helper()
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req, _ := http.NewRequest(method, a.srv.URL+path, rd)
	req.Header.Set("X-Nickname", url.QueryEscape(as))
	req.Header.Set("Content-Type", "application/json")
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer res.Body.Close()
	if out != nil {
		_ = json.NewDecoder(res.Body).Decode(out)
	}
	return res.StatusCode
}

func (a *api) exec(sql string, args ...any) {
	a.t.Helper()
	if _, err := a.db.Exec(context.Background(), sql, args...); err != nil {
		a.t.Fatal(err)
	}
}

func (a *api) folder(name string, parent *string) string {
	a.t.Helper()
	var out struct{ ID string }
	if code := a.do("ann", "POST", "/api/folders", map[string]any{"name": name, "parentId": parent}, &out); code != 201 {
		a.t.Fatalf("create folder %s: %d", name, code)
	}
	return out.ID
}

func TestFolders(t *testing.T) {
	a := newAPI(t)
	show := a.folder("Show", nil)
	s1 := a.folder("Season 1", &show)
	s2 := a.folder("Season 2", &show)

	if code := a.do("ann", "POST", "/api/folders", map[string]any{"name": "  "}, nil); code != 400 {
		t.Fatalf("blank name: %d", code)
	}
	missing := "nope"
	if code := a.do("ann", "POST", "/api/folders", map[string]any{"name": "x", "parentId": missing}, nil); code != 400 {
		t.Fatalf("unknown parent: %d", code)
	}
	// moving a folder into its own subfolder would make a cycle
	if code := a.do("ann", "PATCH", "/api/folders/"+show, map[string]any{"parentId": s1}, nil); code != 400 {
		t.Fatalf("cycle: %d", code)
	}
	if code := a.do("ann", "PATCH", "/api/folders/"+show, map[string]any{"parentId": show}, nil); code != 400 {
		t.Fatalf("into itself: %d", code)
	}
	if code := a.do("ann", "PATCH", "/api/folders/"+s2, map[string]any{"parentId": s1, "name": "S2"}, nil); code != 204 {
		t.Fatalf("move + rename: %d", code)
	}

	a.exec(`INSERT INTO media_files (id, uploaded_by, filename, status, raw_key, folder_id)
		VALUES ('m1', 'ann', 'a', 'ready', 'raw/m1/a', $1)`, s1)
	a.exec(`INSERT INTO torrents (id, added_by, folder_id) VALUES ('t1', 'ann', $1)`, s1)

	// deleting Season 1 moves its subfolder, media and torrents up to Show
	if code := a.do("ann", "DELETE", "/api/folders/"+s1, nil, nil); code != 204 {
		t.Fatalf("delete: %d", code)
	}
	var list []FolderItem
	a.do("ann", "GET", "/api/folders", nil, &list)
	if len(list) != 2 {
		t.Fatalf("folders after delete: %+v", list)
	}
	for _, f := range list {
		if f.ID == s2 && (f.ParentID == nil || *f.ParentID != show) {
			t.Fatalf("subfolder not moved up: %+v", f)
		}
	}
	var mediaFolder, torrentFolder string
	_ = a.db.QueryRow(context.Background(), `SELECT folder_id FROM media_files WHERE id='m1'`).Scan(&mediaFolder)
	_ = a.db.QueryRow(context.Background(), `SELECT folder_id FROM torrents WHERE id='t1'`).Scan(&torrentFolder)
	if mediaFolder != show || torrentFolder != show {
		t.Fatalf("contents not moved up: media %s torrent %s", mediaFolder, torrentFolder)
	}
	if code := a.do("ann", "DELETE", "/api/folders/"+s1, nil, nil); code != 404 {
		t.Fatalf("delete twice: %d", code)
	}
}

func TestBulkMoveAndGuards(t *testing.T) {
	a := newAPI(t)
	dst := a.folder("Dest", nil)
	a.exec(`INSERT INTO media_files (id, uploaded_by, filename, status, raw_key) VALUES
		('a', 'ann', 'a', 'ready', 'raw/a/a'),
		('b', 'ann', 'b', 'processing', 'raw/b/b')`)

	var res struct{ Done, Skipped int }
	if code := a.do("ann", "POST", "/api/media/bulk",
		map[string]any{"ids": []string{"a", "b", "zzz"}, "action": "move", "folderId": dst}, &res); code != 200 {
		t.Fatalf("move: %d", code)
	}
	if res.Done != 2 || res.Skipped != 1 {
		t.Fatalf("move result %+v", res)
	}
	if code := a.do("ann", "POST", "/api/media/bulk",
		map[string]any{"ids": []string{"a"}, "action": "move", "folderId": "missing"}, nil); code != 400 {
		t.Fatalf("move to a missing folder: %d", code)
	}
	if code := a.do("ann", "POST", "/api/media/bulk",
		map[string]any{"ids": []string{"a"}, "action": "explode"}, nil); code != 400 {
		t.Fatalf("unknown action: %d", code)
	}
	if code := a.do("ann", "POST", "/api/media/bulk", map[string]any{"ids": []string{}, "action": "move"}, nil); code != 400 {
		t.Fatalf("no ids: %d", code)
	}

	// reprocess only queues media that are ready/failed and still have the original
	a.exec(`UPDATE media_files SET raw_deleted = true WHERE id='a'`)
	a.do("ann", "POST", "/api/media/bulk", map[string]any{"ids": []string{"a", "b"}, "action": "reprocess"}, &res)
	if res.Done != 0 {
		t.Fatalf("reprocess should skip both: %+v", res)
	}
	if code := a.do("ann", "POST", "/api/media/a/reprocess", nil, nil); code != 409 {
		t.Fatalf("reprocess without original: %d", code)
	}
}

func TestPrivateRoomAccess(t *testing.T) {
	a := newAPI(t)
	var created struct{ ID, InviteCode string }
	if code := a.do("ann", "POST", "/api/rooms",
		map[string]any{"name": "Secret", "private": true, "password": "hunter2"}, &created); code != 201 {
		t.Fatalf("create: %d", code)
	}
	if created.InviteCode == "" {
		t.Fatal("no invite code for a private room")
	}

	// listed only for the owner
	var list []RoomItem
	a.do("bob", "GET", "/api/rooms", nil, &list)
	if len(list) != 0 {
		t.Fatalf("private room listed for others: %+v", list)
	}
	a.do("ann", "GET", "/api/rooms", nil, &list)
	if len(list) != 1 || !list[0].Private {
		t.Fatalf("owner's list: %+v", list)
	}

	// without the invite only the name comes back
	var locked map[string]any
	a.do("bob", "GET", "/api/rooms/"+created.ID, nil, &locked)
	if locked["locked"] != true || locked["ownerName"] != nil || locked["hasPassword"] != true {
		t.Fatalf("locked view: %+v", locked)
	}
	var full RoomItem
	a.do("bob", "GET", "/api/rooms/"+created.ID, nil, &full, "X-Room-Invite", created.InviteCode)
	if full.OwnerName != "ann" || full.InviteCode != nil {
		t.Fatalf("with invite: %+v", full)
	}
	a.do("ann", "GET", "/api/rooms/"+created.ID, nil, &full)
	if full.InviteCode == nil || *full.InviteCode != created.InviteCode {
		t.Fatalf("owner should get the invite code: %+v", full)
	}

	// the password buys the invite code
	if code := a.do("bob", "POST", "/api/rooms/"+created.ID+"/access", map[string]any{"password": "nope"}, nil); code != 403 {
		t.Fatalf("wrong password: %d", code)
	}
	var access struct{ InviteCode string }
	if code := a.do("bob", "POST", "/api/rooms/"+created.ID+"/access", map[string]any{"password": "hunter2"}, &access); code != 200 || access.InviteCode != created.InviteCode {
		t.Fatalf("password: %d %+v", code, access)
	}

	// only the owner changes settings; a new link replaces the old one
	if code := a.do("bob", "PATCH", "/api/rooms/"+created.ID, map[string]any{"resetInvite": true}, nil); code != 403 {
		t.Fatalf("non-owner patch: %d", code)
	}
	if code := a.do("ann", "PATCH", "/api/rooms/"+created.ID, map[string]any{"resetInvite": true}, nil); code != 204 {
		t.Fatalf("reset invite: %d", code)
	}
	a.do("bob", "GET", "/api/rooms/"+created.ID, nil, &locked, "X-Room-Invite", created.InviteCode)
	if locked["locked"] != true {
		t.Fatal("the old invite still works after a reset")
	}
}

func TestHandOverControl(t *testing.T) {
	a := newAPI(t)
	var created struct{ ID string }
	a.do("ann", "POST", "/api/rooms", map[string]any{"name": "Party"}, &created)

	if code := a.do("ann", "PATCH", "/api/rooms/"+created.ID,
		map[string]any{"controllers": []string{"bob", " bob ", "ann", "cat"}}, nil); code != 204 {
		t.Fatalf("set controllers: %d", code)
	}
	var rm RoomItem
	a.do("ann", "GET", "/api/rooms/"+created.ID, nil, &rm)
	if len(rm.Controllers) != 2 || rm.Controllers[0] != "bob" || rm.Controllers[1] != "cat" {
		t.Fatalf("controllers should be cleaned, deduplicated and exclude the owner: %v", rm.Controllers)
	}

	if code := a.do("ann", "PATCH", "/api/rooms/"+created.ID, map[string]any{"ownerName": "bob"}, nil); code != 204 {
		t.Fatalf("hand over: %d", code)
	}
	if code := a.do("ann", "PATCH", "/api/rooms/"+created.ID, map[string]any{"name": "Mine"}, nil); code != 403 {
		t.Fatalf("old owner can still change settings: %d", code)
	}
	a.do("bob", "GET", "/api/rooms/"+created.ID, nil, &rm)
	if rm.OwnerName != "bob" {
		t.Fatalf("owner: %s", rm.OwnerName)
	}
}
