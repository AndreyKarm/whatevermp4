package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"liotom/whateverserver/internal/auth"
	"liotom/whateverserver/internal/jobs"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type TorrentItem struct {
	ID         string    `json:"id"`
	UserID     string    `json:"userId"`
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	Progress   float64   `json:"progress"`
	BytesDone  int64     `json:"bytesDone"`
	BytesTotal int64     `json:"bytesTotal"`
	Error      *string   `json:"error,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}

func (s *Server) listTorrents(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(r.Context(), `
		SELECT id, user_id, name, status, progress, bytes_done, bytes_total, error, created_at
		FROM torrents
		ORDER BY created_at DESC`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	items := []TorrentItem{}
	for rows.Next() {
		var t TorrentItem
		if err := rows.Scan(
			&t.ID, &t.UserID, &t.Name, &t.Status, &t.Progress,
			&t.BytesDone, &t.BytesTotal, &t.Error, &t.CreatedAt,
		); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		items = append(items, t)
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) createTorrent(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Magnet   string `json:"magnet"`
		Metainfo string `json:"metainfo"` // base64-encoded .torrent file
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}

	var magnet *string
	var meta []byte
	if m := strings.TrimSpace(in.Magnet); strings.HasPrefix(m, "magnet:?") {
		magnet = &m
	} else if in.Metainfo != "" {
		b, err := base64.StdEncoding.DecodeString(in.Metainfo)
		if err != nil || len(b) == 0 {
			writeErr(w, http.StatusBadRequest, "invalid metainfo base64")
			return
		}
		meta = b
	} else {
		writeErr(w, http.StatusBadRequest, "provide a magnet link or metainfo file")
		return
	}

	user := auth.FromContext(r.Context())
	id := uuid.NewString()
	_, err := s.db.Exec(r.Context(), `
		INSERT INTO torrents (id, user_id, name, magnet, metainfo, status)
		VALUES ($1, $2, 'Fetching metadata...', $3, $4, 'queued')`,
		id, user.ID, magnet, meta)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := jobs.Enqueue(r.Context(), s.db, "torrent", map[string]string{"torrentId": id}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (s *Server) cancelTorrent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var userID string
	err := s.db.QueryRow(r.Context(), `SELECT user_id FROM torrents WHERE id=$1`, id).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "torrent not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !s.canManage(r, userID) {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}

	_, _ = s.db.Exec(r.Context(), `UPDATE torrents SET status='cancelled' WHERE id=$1`, id)
	w.WriteHeader(http.StatusNoContent)
}
