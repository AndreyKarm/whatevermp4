package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"liotom/whateverserver/internal/jobs"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type TorrentItem struct {
	ID         string    `json:"id"`
	AddedBy    string    `json:"addedBy"`
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
		SELECT id, added_by, name, status, progress, bytes_done, bytes_total, error, created_at
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
			&t.ID, &t.AddedBy, &t.Name, &t.Status, &t.Progress,
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
		Magnet   string  `json:"magnet"`
		Metainfo string  `json:"metainfo"` // base64-encoded .torrent file
		FolderID *string `json:"folderId"` // where the imported media go
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

	id := uuid.NewString()
	_, err := s.db.Exec(r.Context(), `
		INSERT INTO torrents (id, added_by, name, magnet, metainfo, status, folder_id)
		VALUES ($1, $2, 'Fetching metadata...', $3, $4, 'queued', $5)`,
		id, nickname(r), magnet, meta, emptyToNil(in.FolderID))
	if isForeignKeyViolation(err) {
		writeErr(w, http.StatusBadRequest, "folder not found")
		return
	}
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
	tag, err := s.db.Exec(r.Context(),
		// cancelling a failed torrent just dismisses it from the downloads list
		`UPDATE torrents SET status='cancelled' WHERE id=$1 AND status IN ('queued','downloading','failed')`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "torrent not found or already finished")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
