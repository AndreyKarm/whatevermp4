package social

import (
	"context"
	"encoding/json"
	"net/http"

	"movie-platform/shared/authmw"
	"movie-platform/shared/httpx"
)

type RatingsRepository struct {
	db DBTX
}

func NewRatingsRepository(db DBTX) *RatingsRepository {
	return &RatingsRepository{db: db}
}

func (r *RatingsRepository) Upsert(ctx context.Context, userID, titleID string, score int16) error {
	_, err := r.db.Exec(ctx,
		`insert into ratings (user_id, title_id, score) values ($1, $2, $3)
		 on conflict (user_id, title_id) do update set score = excluded.score, updated_at = now()`,
		userID, titleID, score,
	)
	return err
}

type RatingsHandler struct {
	repo *RatingsRepository
}

func NewRatingsHandler(repo *RatingsRepository) *RatingsHandler {
	return &RatingsHandler{repo: repo}
}

func (h *RatingsHandler) Upsert(w http.ResponseWriter, r *http.Request) {
	claims, ok := authmw.ClaimsFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var body struct {
		TitleID string `json:"titleId"`
		Score   int16  `json:"score"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Score < 1 || body.Score > 10 {
		httpx.Error(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := h.repo.Upsert(r.Context(), claims.UserID, body.TitleID, body.Score); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed to save rating")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
