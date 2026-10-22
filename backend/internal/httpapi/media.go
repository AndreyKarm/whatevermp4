package httpapi

import (
	"encoding/json"
	"errors"
	"liotom/whateverserver/internal/jobs"
	"liotom/whateverserver/internal/storage"
	"net/http"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type MediaItem struct {
	ID             string          `json:"id"`
	UploadedBy     string          `json:"uploadedBy"`
	TorrentID      *string         `json:"torrentId,omitempty"`
	FolderID       *string         `json:"folderId,omitempty"`
	Filename       string          `json:"filename"`
	Status         string          `json:"status"`
	HLSKey         *string         `json:"hlsKey,omitempty"`
	PosterKey      *string         `json:"posterKey,omitempty"`
	PlaybackURL    string          `json:"playbackUrl,omitempty"`
	DurationSec    *float64        `json:"durationSec,omitempty"`
	Season         *int            `json:"season,omitempty"`
	Episode        *int            `json:"episode,omitempty"`
	AudioTracks    json.RawMessage `json:"audioTracks"`
	SubtitleTracks json.RawMessage `json:"subtitleTracks"`
	Fonts          json.RawMessage `json:"fonts"`
	Error          *string         `json:"error,omitempty"`
	CreatedAt      time.Time       `json:"createdAt"`
}

// opt distinguishes an omitted JSON field (Set=false) from an explicit null.
type opt[T any] struct {
	Set bool
	V   *T
}

func (o *opt[T]) UnmarshalJSON(b []byte) error {
	o.Set = true
	return json.Unmarshal(b, &o.V)
}

const mediaColumns = `id, uploaded_by, torrent_id, folder_id, filename, status, hls_key, poster_key,
	duration_sec, season, episode, audio_tracks, subtitle_tracks, fonts, error, created_at`

// scan reads a row selected with mediaColumns.
func (m *MediaItem) scan(row pgx.Row) error {
	return row.Scan(
		&m.ID, &m.UploadedBy, &m.TorrentID, &m.FolderID, &m.Filename, &m.Status,
		&m.HLSKey, &m.PosterKey, &m.DurationSec, &m.Season, &m.Episode,
		&m.AudioTracks, &m.SubtitleTracks, &m.Fonts, &m.Error, &m.CreatedAt,
	)
}

func (s *Server) listMedia(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(r.Context(),
		`SELECT `+mediaColumns+` FROM media_files ORDER BY created_at DESC`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	items := []MediaItem{}
	for rows.Next() {
		var m MediaItem
		if err := m.scan(rows); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if m.Status == "ready" {
			m.PlaybackURL = s.playbackBase(m.ID)
		}
		items = append(items, m)
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) getMedia(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var m MediaItem
	err := m.scan(s.db.QueryRow(r.Context(),
		`SELECT `+mediaColumns+` FROM media_files WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "media not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if m.Status == "ready" {
		m.PlaybackURL = s.playbackBase(m.ID)
	}
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) updateMedia(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var in struct {
		Filename *string     `json:"filename"`
		Season   opt[int]    `json:"season"`
		Episode  opt[int]    `json:"episode"`
		FolderID opt[string] `json:"folderId"` // null = top level
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.FolderID.Set {
		in.FolderID.V = emptyToNil(in.FolderID.V)
	}
	if in.Filename != nil {
		name := strings.TrimSpace(*in.Filename)
		if name == "" || utf8.RuneCountInString(name) > 255 {
			writeErr(w, http.StatusBadRequest, "filename must be 1-255 characters")
			return
		}
		in.Filename = &name
		// A rename that names an episode updates season/episode too,
		// unless the caller set them explicitly.
		if !in.Season.Set && !in.Episode.Set {
			if season, ep := storage.ParseSE(name); season != nil {
				in.Season, in.Episode = opt[int]{true, season}, opt[int]{true, ep}
			}
		}
	}
	// season/episode/folderId: omitted = unchanged, null = cleared
	tag, err := s.db.Exec(r.Context(), `
		UPDATE media_files
		SET filename  = COALESCE($2, filename),
		    season    = CASE WHEN $3::boolean THEN $4::integer ELSE season END,
		    episode   = CASE WHEN $5::boolean THEN $6::integer ELSE episode END,
		    folder_id = CASE WHEN $7::boolean THEN $8::text ELSE folder_id END
		WHERE id = $1`, id, in.Filename, in.Season.Set, in.Season.V, in.Episode.Set, in.Episode.V,
		in.FolderID.Set, in.FolderID.V)
	if isForeignKeyViolation(err) {
		writeErr(w, http.StatusBadRequest, "folder not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "media not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/media/{id}/download redirects to a presigned link for the original file.
func (s *Server) downloadMedia(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var filename, rawKey, status string
	err := s.db.QueryRow(r.Context(),
		`SELECT filename, raw_key, status FROM media_files WHERE id=$1`, id).
		Scan(&filename, &rawKey, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "media not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if status == "uploading" || status == "downloading" {
		writeErr(w, http.StatusConflict, "the original file isn't stored yet")
		return
	}
	// keep the stored container's extension (e.g. imports are .mkv)
	if ext := path.Ext(rawKey); !strings.EqualFold(path.Ext(filename), ext) {
		filename += ext
	}
	u, err := s.st.PresignDownload(r.Context(), rawKey, filename)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	http.Redirect(w, r, u, http.StatusFound)
}

// POST /api/media/{id}/reprocess re-runs the transcode from the stored original.
func (s *Server) reprocessMedia(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	tag, err := s.db.Exec(r.Context(), `
		UPDATE media_files SET status='processing', error=NULL
		WHERE id=$1 AND status IN ('ready','failed')`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, http.StatusConflict, "media not found or still being prepared")
		return
	}
	if err := jobs.Enqueue(r.Context(), s.db, "process", map[string]string{"mediaId": id}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) deleteMedia(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var rawKey string
	var hlsKey *string
	err := s.db.QueryRow(r.Context(),
		`SELECT raw_key, hls_key FROM media_files WHERE id=$1`, id).
		Scan(&rawKey, &hlsKey)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "media not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	_, _ = s.db.Exec(r.Context(), `DELETE FROM media_files WHERE id=$1`, id)
	_ = s.st.DeleteKey(r.Context(), rawKey)
	if hlsKey != nil {
		_ = s.st.DeletePrefix(r.Context(), *hlsKey)
	}
	w.WriteHeader(http.StatusNoContent)
}
