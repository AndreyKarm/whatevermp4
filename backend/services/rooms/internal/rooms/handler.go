package rooms

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"movie-platform/shared/authmw"
	"movie-platform/shared/httpx"
)

type Handler struct {
	repo *Repository
}

func NewHandler(repo *Repository) *Handler {
	return &Handler{repo: repo}
}

func (h *Handler) CreateRoom(w http.ResponseWriter, r *http.Request) {
	claims, ok := authmw.ClaimsFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var body struct {
		MediaVersionID string `json:"mediaVersionId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.MediaVersionID == "" {
		httpx.Error(w, http.StatusBadRequest, "invalid body")
		return
	}
	room, err := h.repo.CreateRoom(r.Context(), claims.UserID, body.MediaVersionID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed to create room")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, room)
}

func (h *Handler) JoinRoom(w http.ResponseWriter, r *http.Request) {
	claims, ok := authmw.ClaimsFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	code := chi.URLParam(r, "code")
	room, err := h.repo.GetRoomByCode(r.Context(), code)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, "room not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "failed to join room")
		return
	}
	if err := h.repo.AddMember(r.Context(), room.ID, claims.UserID); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed to join room")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, room)
}

func (h *Handler) UpdatePlayback(w http.ResponseWriter, r *http.Request) {
	roomID := chi.URLParam(r, "id")
	var body struct {
		Position  float64 `json:"position"`
		IsPlaying bool    `json:"isPlaying"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := h.repo.UpdatePlaybackState(r.Context(), roomID, body.Position, body.IsPlaying); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed to update playback")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Note: real-time sync between members (play/pause/seek broadcast) needs a
// WebSocket hub here. That's a natural follow-up once REST endpoints work.
