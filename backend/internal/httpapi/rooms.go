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

	items := []RoomItem{}
	for rows.Next() {
		var rm RoomItem
		if err := rows.Scan(
			&rm.ID, &rm.Name, &rm.OwnerName, &rm.EveryoneControls,
			&rm.State, &rm.UpdatedAt, &rm.CreatedAt,
		); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		items = append(items, rm)
	}
	writeJSON(w, http.StatusOK, items)
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
