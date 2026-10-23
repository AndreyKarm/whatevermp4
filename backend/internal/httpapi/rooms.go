package httpapi

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"liotom/whateverserver/internal/rooms"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

type RoomItem struct {
	ID               string          `json:"id"`
	Name             string          `json:"name"`
	OwnerName        string          `json:"ownerName"`
	EveryoneControls bool            `json:"everyoneControls"`
	Controllers      []string        `json:"controllers"`
	Private          bool            `json:"private"`
	HasPassword      bool            `json:"hasPassword"`
	State            json.RawMessage `json:"state"`
	UpdatedAt        time.Time       `json:"updatedAt"`
	CreatedAt        time.Time       `json:"createdAt"`
	// getRoom only, for the owner of a private room: the code invite links carry
	InviteCode *string `json:"inviteCode,omitempty"`
	// listRooms only: who is connected right now and what is selected
	Members []string   `json:"members,omitempty"`
	Media   *RoomMedia `json:"media,omitempty"`
}

type RoomMedia struct {
	ID        string `json:"id"`
	Filename  string `json:"filename"`
	PosterURL string `json:"posterUrl,omitempty"`
}

// roomRow is the full rooms row, including the secrets RoomItem leaves out.
type roomRow struct {
	RoomItem
	passwordHash *string
	inviteCode   *string
}

const roomColumns = `id, name, owner_name, everyone_controls, controllers, private,
	password_hash, invite_code, state, updated_at, created_at`

func (rm *roomRow) scan(row pgx.Row) error {
	err := row.Scan(&rm.ID, &rm.Name, &rm.OwnerName, &rm.EveryoneControls, &rm.Controllers,
		&rm.Private, &rm.passwordHash, &rm.inviteCode, &rm.State, &rm.UpdatedAt, &rm.CreatedAt)
	rm.HasPassword = rm.passwordHash != nil
	return err
}

func (rm *roomRow) access() rooms.Access {
	a := rooms.Access{Name: rm.Name, Owner: rm.OwnerName, Everyone: rm.EveryoneControls,
		Controllers: rm.Controllers, Private: rm.Private, HasPassword: rm.passwordHash != nil}
	if rm.inviteCode != nil {
		a.InviteCode = *rm.inviteCode
	}
	return a
}

// newInviteCode is the secret a private room's invite links carry.
func newInviteCode() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func hashPassword(pw string) (*string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	s := string(h)
	return &s, nil
}

// roomPassword validates a new password; "" means none.
func roomPassword(pw string) (string, bool) {
	return pw, len(pw) <= 72 // bcrypt's limit, in bytes
}

const errPasswordTooLong = "the password is too long (at most 72 bytes)"

func (s *Server) listRooms(w http.ResponseWriter, r *http.Request) {
	// private rooms are listed only for their owner
	rows, err := s.db.Query(r.Context(), `SELECT `+roomColumns+` FROM rooms
		WHERE NOT private OR owner_name = $1
		ORDER BY updated_at DESC`, nickname(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	live := s.hub.Live()
	items := []RoomItem{}
	mediaIDs := map[string]bool{}
	for rows.Next() {
		var row roomRow
		if err := row.scan(rows); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		rm := row.RoomItem
		// The DB copy of state is flushed every 10s; prefer the hub's.
		if l, ok := live[rm.ID]; ok {
			rm.Members = l.Members
			rm.State, _ = json.Marshal(l.State)
		}
		var st rooms.State
		if json.Unmarshal(rm.State, &st) == nil && st.MediaID != "" {
			rm.Media = &RoomMedia{ID: st.MediaID}
			mediaIDs[st.MediaID] = true
		}
		items = append(items, rm)
	}
	if err := rows.Err(); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.fillRoomMedia(r, items, mediaIDs); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// fillRoomMedia adds filename and poster URL to each room's selected media.
func (s *Server) fillRoomMedia(r *http.Request, items []RoomItem, ids map[string]bool) error {
	if len(ids) == 0 {
		return nil
	}
	list := make([]string, 0, len(ids))
	for id := range ids {
		list = append(list, id)
	}
	rows, err := s.db.Query(r.Context(), `
		SELECT id, filename, status, poster_key FROM media_files WHERE id = ANY($1)`, list)
	if err != nil {
		return err
	}
	defer rows.Close()
	found := map[string]RoomMedia{}
	for rows.Next() {
		var m RoomMedia
		var status string
		var poster *string
		if err := rows.Scan(&m.ID, &m.Filename, &status, &poster); err != nil {
			return err
		}
		if status == "ready" && poster != nil {
			m.PosterURL = s.playbackBase(m.ID) + "/poster.jpg"
		}
		found[m.ID] = m
	}
	for i := range items {
		if items[i].Media == nil {
			continue
		}
		if m, ok := found[items[i].Media.ID]; ok {
			items[i].Media = &m
		} else {
			items[i].Media = nil // media was deleted
		}
	}
	return rows.Err()
}

func (s *Server) createRoom(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name             string  `json:"name"`
		MediaID          string  `json:"mediaId"`
		EveryoneControls bool    `json:"everyoneControls"`
		Position         float64 `json:"position"` // start where the host last watched
		Private          bool    `json:"private"`
		Password         string  `json:"password"` // private rooms only; "" = invite link only
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = "Watch Party"
	}
	if in.MediaID != "" {
		var status string
		err := s.db.QueryRow(r.Context(),
			`SELECT status FROM media_files WHERE id=$1`, in.MediaID).Scan(&status)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && status != "ready") {
			writeErr(w, http.StatusBadRequest, "media not found or not ready")
			return
		}
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	var invite, hash *string
	if in.Private {
		code := newInviteCode()
		invite = &code
		pw, ok := roomPassword(in.Password)
		if !ok {
			writeErr(w, http.StatusBadRequest, errPasswordTooLong)
			return
		}
		if pw != "" {
			var err error
			if hash, err = hashPassword(pw); err != nil {
				writeErr(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
	}

	id := uuid.NewString()
	st, _ := json.Marshal(rooms.State{
		MediaID:   in.MediaID,
		Playing:   false,
		Position:  max(in.Position, 0),
		UpdatedAt: time.Now().UnixMilli(),
		Rate:      1,
	})

	_, err := s.db.Exec(r.Context(), `
		INSERT INTO rooms (id, name, owner_name, everyone_controls, state, private, password_hash, invite_code)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		id, name, nickname(r), in.EveryoneControls, st, in.Private, hash, invite)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := map[string]string{"id": id}
	if invite != nil {
		out["inviteCode"] = *invite
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) loadRoom(w http.ResponseWriter, r *http.Request, id string) (*roomRow, bool) {
	var rm roomRow
	err := rm.scan(s.db.QueryRow(r.Context(), `SELECT `+roomColumns+` FROM rooms WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "room not found")
		return nil, false
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return nil, false
	}
	return &rm, true
}

// GET /api/rooms/{id}. A private room needs its invite code in X-Room-Invite
// (or the owner's nickname); otherwise only its name comes back, marked
// locked, so the page can ask for the password.
func (s *Server) getRoom(w http.ResponseWriter, r *http.Request) {
	rm, ok := s.loadRoom(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	owner := rm.OwnerName == nickname(r)
	if rm.Private && !owner && !inviteMatches(rm.inviteCode, r.Header.Get("X-Room-Invite")) {
		writeJSON(w, http.StatusOK, map[string]any{
			"id": rm.ID, "name": rm.Name, "private": true, "locked": true,
			"hasPassword": rm.HasPassword,
		})
		return
	}
	if owner && rm.Private {
		rm.InviteCode = rm.inviteCode
	}
	writeJSON(w, http.StatusOK, rm.RoomItem)
}

func inviteMatches(code *string, given string) bool {
	return code != nil && given != "" && subtle.ConstantTimeCompare([]byte(*code), []byte(given)) == 1
}

// POST /api/rooms/{id}/access {password} trades a private room's password
// for its invite code, which the WebSocket needs.
func (s *Server) roomAccess(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	rm, ok := s.loadRoom(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	if !rm.Private || rm.inviteCode == nil {
		writeErr(w, http.StatusBadRequest, "this room isn't private")
		return
	}
	if rm.passwordHash == nil ||
		bcrypt.CompareHashAndPassword([]byte(*rm.passwordHash), []byte(in.Password)) != nil {
		writeErr(w, http.StatusForbidden, "wrong password")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"inviteCode": *rm.inviteCode})
}

// PATCH /api/rooms/{id} (owner only). ownerName hands the room to someone
// else; controllers lists nicknames who may control playback; password ""
// removes it; resetInvite makes old invite links stop working.
func (s *Server) updateRoom(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var in struct {
		Name             *string   `json:"name"`
		EveryoneControls *bool     `json:"everyoneControls"`
		OwnerName        *string   `json:"ownerName"`
		Controllers      *[]string `json:"controllers"`
		Private          *bool     `json:"private"`
		Password         *string   `json:"password"`
		ResetInvite      bool      `json:"resetInvite"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	rm, ok := s.loadRoom(w, r, id)
	if !ok {
		return
	}
	if rm.OwnerName != nickname(r) {
		writeErr(w, http.StatusForbidden, "only the room owner can do that")
		return
	}

	if in.Name != nil {
		if n := strings.TrimSpace(*in.Name); n != "" {
			rm.Name = n
		}
	}
	if in.EveryoneControls != nil {
		rm.EveryoneControls = *in.EveryoneControls
	}
	if in.OwnerName != nil {
		rm.OwnerName = rooms.CleanNickname(*in.OwnerName)
	}
	if in.Controllers != nil {
		var list []string
		for _, n := range *in.Controllers {
			if n = rooms.CleanNickname(n); n != rm.OwnerName && !slices.Contains(list, n) && len(list) < 50 {
				list = append(list, n)
			}
		}
		rm.Controllers = list
	}
	if rm.Controllers == nil {
		rm.Controllers = []string{}
	}
	if in.Private != nil {
		rm.Private = *in.Private
	}
	if in.Password != nil {
		pw, ok := roomPassword(*in.Password)
		if !ok {
			writeErr(w, http.StatusBadRequest, errPasswordTooLong)
			return
		}
		rm.passwordHash = nil
		if pw != "" {
			var err error
			if rm.passwordHash, err = hashPassword(pw); err != nil {
				writeErr(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
	}
	if rm.Private && (rm.inviteCode == nil || in.ResetInvite) {
		code := newInviteCode()
		rm.inviteCode = &code
	}

	controllers, _ := json.Marshal(rm.Controllers)
	if _, err := s.db.Exec(r.Context(), `
		UPDATE rooms
		SET name = $2, everyone_controls = $3, owner_name = $4, controllers = $5,
		    private = $6, password_hash = $7, invite_code = $8, updated_at = now()
		WHERE id = $1`, id, rm.Name, rm.EveryoneControls, rm.OwnerName, controllers,
		rm.Private, rm.passwordHash, rm.inviteCode); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.hub.SetAccess(id, rm.access())
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteRoom(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !s.requireRoomOwner(w, r, id) {
		return
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM rooms WHERE id=$1`, id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.hub.Remove(id)
	w.WriteHeader(http.StatusNoContent)
}

// requireRoomOwner writes 404/403 and returns false unless the caller's
// nickname matches the room owner.
func (s *Server) requireRoomOwner(w http.ResponseWriter, r *http.Request, id string) bool {
	var owner string
	err := s.db.QueryRow(r.Context(), `SELECT owner_name FROM rooms WHERE id=$1`, id).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "room not found")
		return false
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return false
	}
	if owner != nickname(r) {
		writeErr(w, http.StatusForbidden, "only the room owner can do that")
		return false
	}
	return true
}
