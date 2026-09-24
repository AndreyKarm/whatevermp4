package social

import (
	"context"
	"encoding/json"
	"net/http"

	"movie-platform/shared/authmw"
	"movie-platform/shared/httpx"
)

type WatchlistRepository struct {
	db DBTX
}

func NewWatchlistRepository(db DBTX) *WatchlistRepository {
	return &WatchlistRepository{db: db}
}

func (r *WatchlistRepository) Upsert(ctx context.Context, userID, titleID, status string) error {
	_, err := r.db.Exec(ctx,
		`insert into watchlist_items (user_id, title_id, status) values ($1, $2, $3)
		 on conflict (user_id, title_id) do update set status = excluded.status`,
		userID, titleID, status,
	)
	return err
}

type WatchlistHandler struct {
	repo *WatchlistRepository
}

func NewWatchlistHandler(repo *WatchlistRepository) *WatchlistHandler {
	return &WatchlistHandler{repo: repo}
}

func (h *WatchlistHandler) Upsert(w http.ResponseWriter, r *http.Request) {
	claims, ok := authmw.ClaimsFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var body struct {
		TitleID string `json:"titleId"`
		Status  string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid body")
		return
	}
	switch body.Status {
	case "planned", "watching", "completed", "dropped":
	default:
		httpx.Error(w, http.StatusBadRequest, "invalid status")
		return
	}
	if err := h.repo.Upsert(r.Context(), claims.UserID, body.TitleID, body.Status); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed to update watchlist")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
