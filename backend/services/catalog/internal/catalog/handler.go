package catalog

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

func (h *Handler) ListTitles(w http.ResponseWriter, r *http.Request) {
	titles, err := h.repo.ListTitles(r.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed to list titles")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, titles)
}

func (h *Handler) GetTitle(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	t, err := h.repo.GetTitle(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, "title not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "failed to get title")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, t)
}

func (h *Handler) CreateTitle(w http.ResponseWriter, r *http.Request) {
	claims, ok := authmw.ClaimsFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	var body struct {
		Type        string  `json:"type"`
		Name        string  `json:"name"`
		Year        *int    `json:"year"`
		Description *string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Type != "movie" && body.Type != "series" {
		httpx.Error(w, http.StatusBadRequest, "invalid type")
		return
	}

	t, err := h.repo.CreateTitle(r.Context(), body.Type, body.Name, body.Year, body.Description, claims.UserID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed to create title")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, t)
}

// UpdateMediaVersionStatus is called internally by the Processing service,
// authenticated with a short-lived internal service token (see
// authmw.RequireServiceAuth) rather than an end-user JWT.
func (h *Handler) UpdateMediaVersionStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		Status            string  `json:"status"`
		MasterManifestKey *string `json:"masterManifestKey"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := h.repo.UpdateMediaVersionStatus(r.Context(), id, body.Status, body.MasterManifestKey); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "update failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
