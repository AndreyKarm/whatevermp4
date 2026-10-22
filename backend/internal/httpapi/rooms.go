package httpapi

import (
	"encoding/json"
	"errors"
	"liotom/whateverserver/internal/rooms"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type RoomItem struct {
	ID               string          `json:"id"`
	Name             string          `json:"name"`
	OwnerName        string          `json:"ownerName"`
	EveryoneControls bool            `json:"everyoneControls"`
	State            json.RawMessage `json:"state"`
	UpdatedAt        time.Time       `json:"updatedAt"`
	CreatedAt        time.Time       `json:"createdAt"`
	// listRooms only: who is connected right now and what is selected
	Members []string   `json:"members,omitempty"`
	Media   *RoomMedia `json:"media,omitempty"`
}

type RoomMedia struct {
	ID        string `json:"id"`
	Filename  string `json:"filename"`
	PosterURL string `json:"posterUrl,omitempty"`
}

func (s *Server) listRooms(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(r.Context(), `
		SELECT id, name, owner_name, everyone_controls, state, updated_at, created_at
		FROM rooms
		ORDER BY updated_at DESC`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	live := s.hub.Live()
	items := []RoomItem{}
	mediaIDs := map[string]bool{}
	for rows.Next() {
		var rm RoomItem
		if err := rows.Scan(
			&rm.ID, &rm.Name, &rm.OwnerName, &rm.EveryoneControls,
			&rm.State, &rm.UpdatedAt, &rm.CreatedAt,
		); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
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
		Name             string `json:"name"`
		MediaID          string `json:"mediaId"`
		EveryoneControls bool   `json:"everyoneControls"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = "Watch Party"
	}

	id := uuid.NewString()
	st, _ := json.Marshal(rooms.State{
		MediaID:   in.MediaID,
		Playing:   false,
		Position:  0,
		UpdatedAt: time.Now().UnixMilli(),
		Rate:      1,
	})

	_, err := s.db.Exec(r.Context(), `
		INSERT INTO rooms (id, name, owner_name, everyone_controls, state)
		VALUES ($1, $2, $3, $4, $5)`,
		id, name, nickname(r), in.EveryoneControls, st)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (s *Server) getRoom(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var rm RoomItem
	err := s.db.QueryRow(r.Context(), `
		SELECT id, name, owner_name, everyone_controls, state, updated_at, created_at
		FROM rooms WHERE id=$1`, id).Scan(
		&rm.ID, &rm.Name, &rm.OwnerName, &rm.EveryoneControls,
		&rm.State, &rm.UpdatedAt, &rm.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "room not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rm)
}

func (s *Server) updateRoom(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var in struct {
		Name             *string `json:"name"`
		EveryoneControls *bool   `json:"everyoneControls"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if !s.requireRoomOwner(w, r, id) {
		return
	}
	var everyone bool
	err := s.db.QueryRow(r.Context(), `
		UPDATE rooms
		SET name              = COALESCE($2, name),
		    everyone_controls = COALESCE($3, everyone_controls),
		    updated_at        = now()
		WHERE id = $1
		RETURNING everyone_controls`, id, in.Name, in.EveryoneControls).Scan(&everyone)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.hub.SetEveryoneControls(id, everyone)
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
