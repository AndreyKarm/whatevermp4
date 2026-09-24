package social

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"movie-platform/shared/authmw"
	"movie-platform/shared/httpx"
)

type Comment struct {
	ID              string    `json:"id"`
	UserID          string    `json:"userId"`
	TitleID         *string   `json:"titleId,omitempty"`
	EpisodeID       *string   `json:"episodeId,omitempty"`
	ParentCommentID *string   `json:"parentCommentId,omitempty"`
	Body            string    `json:"body"`
	CreatedAt       time.Time `json:"createdAt"`
}

type CommentsRepository struct {
	db DBTX
}

func NewCommentsRepository(db DBTX) *CommentsRepository {
	return &CommentsRepository{db: db}
}

func (r *CommentsRepository) Create(ctx context.Context, userID string, titleID, episodeID, parentID *string, body string) (*Comment, error) {
	c := &Comment{UserID: userID, TitleID: titleID, EpisodeID: episodeID, ParentCommentID: parentID, Body: body}
	err := r.db.QueryRow(ctx,
		`insert into comments (user_id, title_id, episode_id, parent_comment_id, body)
		 values ($1, $2, $3, $4, $5)
		 returning id, created_at`,
		userID, titleID, episodeID, parentID, body,
	).Scan(&c.ID, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (r *CommentsRepository) ListByTitle(ctx context.Context, titleID string) ([]Comment, error) {
	rows, err := r.db.Query(ctx,
		`select id, user_id, title_id, episode_id, parent_comment_id, body, created_at
		 from comments where title_id = $1 and deleted_at is null order by created_at asc`,
		titleID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Comment
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.ID, &c.UserID, &c.TitleID, &c.EpisodeID, &c.ParentCommentID, &c.Body, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

type CommentsHandler struct {
	repo *CommentsRepository
}

func NewCommentsHandler(repo *CommentsRepository) *CommentsHandler {
	return &CommentsHandler{repo: repo}
}

func (h *CommentsHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims, ok := authmw.ClaimsFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var body struct {
		TitleID   *string `json:"titleId"`
		EpisodeID *string `json:"episodeId"`
		ParentID  *string `json:"parentCommentId"`
		Body      string  `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Body == "" {
		httpx.Error(w, http.StatusBadRequest, "invalid body")
		return
	}
	c, err := h.repo.Create(r.Context(), claims.UserID, body.TitleID, body.EpisodeID, body.ParentID, body.Body)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed to create comment")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, c)
}

func (h *CommentsHandler) ListByTitle(w http.ResponseWriter, r *http.Request) {
	titleID := chi.URLParam(r, "titleId")
	comments, err := h.repo.ListByTitle(r.Context(), titleID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed to list comments")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, comments)
}
