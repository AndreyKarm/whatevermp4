package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type MediaItem struct {
	ID             string          `json:"id"`
	UploadedBy     string          `json:"uploadedBy"`
	TorrentID      *string         `json:"torrentId,omitempty"`
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

// optInt distinguishes an omitted JSON field (Set=false) from an explicit null.
type optInt struct {
	Set bool
	V   *int
}

func (o *optInt) UnmarshalJSON(b []byte) error {
	o.Set = true
	return json.Unmarshal(b, &o.V)
}

func (s *Server) listMedia(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(r.Context(), `
		SELECT id, uploaded_by, torrent_id, filename, status, hls_key, poster_key,
			duration_sec, season, episode, audio_tracks, subtitle_tracks, fonts, error, created_at
		FROM media_files
		ORDER BY created_at DESC`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	items := []MediaItem{}
	for rows.Next() {
		var m MediaItem
		if err := rows.Scan(
			&m.ID, &m.UploadedBy, &m.TorrentID, &m.Filename, &m.Status,
			&m.HLSKey, &m.PosterKey, &m.DurationSec, &m.Season, &m.Episode,
			&m.AudioTracks, &m.SubtitleTracks, &m.Fonts, &m.Error, &m.CreatedAt,
		); err != nil {
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
	err := s.db.QueryRow(r.Context(), `
		SELECT id, uploaded_by, torrent_id, filename, status, hls_key, poster_key,
			duration_sec, season, episode, audio_tracks, subtitle_tracks, fonts, error, created_at
		FROM media_files WHERE id=$1`, id).Scan(
		&m.ID, &m.UploadedBy, &m.TorrentID, &m.Filename, &m.Status,
		&m.HLSKey, &m.PosterKey, &m.DurationSec, &m.Season, &m.Episode,
		&m.AudioTracks, &m.SubtitleTracks, &m.Fonts, &m.Error, &m.CreatedAt,
	)
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
		Filename *string `json:"filename"`
		Season   optInt  `json:"season"`
		Episode  optInt  `json:"episode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	// season/episode: omitted = unchanged, null = cleared
	tag, err := s.db.Exec(r.Context(), `
		UPDATE media_files
		SET filename = COALESCE($2, filename),
		    season   = CASE WHEN $3::boolean THEN $4::integer ELSE season END,
		    episode  = CASE WHEN $5::boolean THEN $6::integer ELSE episode END
		WHERE id = $1`, id, in.Filename, in.Season.Set, in.Season.V, in.Episode.Set, in.Episode.V)
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
