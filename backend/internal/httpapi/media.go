package httpapi

import (
	"encoding/json"
	"errors"
	"liotom/whateverserver/internal/jobs"
	"liotom/whateverserver/internal/storage"
	"log"
	"net/http"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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
	Thumbnails     json.RawMessage `json:"thumbnails,omitempty"` // timeline sprites, see transcode.Thumbnails
	Progress       *float64        `json:"progress,omitempty"`   // 0..1 while downloading/processing
	RawBytes       *int64          `json:"rawBytes,omitempty"`
	HLSBytes       *int64          `json:"hlsBytes,omitempty"`
	RawDeleted     bool            `json:"rawDeleted,omitempty"`
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
	duration_sec, season, episode, audio_tracks, subtitle_tracks, fonts, thumbnails, progress,
	raw_bytes, hls_bytes, raw_deleted, error, created_at`

// scan reads a row selected with mediaColumns.
func (m *MediaItem) scan(row pgx.Row) error {
	return row.Scan(
		&m.ID, &m.UploadedBy, &m.TorrentID, &m.FolderID, &m.Filename, &m.Status,
		&m.HLSKey, &m.PosterKey, &m.DurationSec, &m.Season, &m.Episode,
		&m.AudioTracks, &m.SubtitleTracks, &m.Fonts, &m.Thumbnails, &m.Progress,
		&m.RawBytes, &m.HLSBytes, &m.RawDeleted, &m.Error, &m.CreatedAt,
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
	var rawDeleted bool
	err := s.db.QueryRow(r.Context(),
		`SELECT filename, raw_key, status, raw_deleted FROM media_files WHERE id=$1`, id).
		Scan(&filename, &rawKey, &status, &rawDeleted)
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
	if rawDeleted {
		writeErr(w, http.StatusConflict, errOriginalDeleted)
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

const errOriginalDeleted = "the original file was deleted"

// POST /api/media/{id}/reprocess re-runs the transcode from the stored original.
func (s *Server) reprocessMedia(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ids, err := s.reprocess(r, []string{id})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(ids) == 0 {
		writeErr(w, http.StatusConflict, "media not found, still being prepared, or its original was deleted")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// reprocess queues a transcode for the given media that are ready or failed
// and still have their original; it returns the IDs it queued.
func (s *Server) reprocess(r *http.Request, ids []string) ([]string, error) {
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(r.Context())
	rows, err := tx.Query(r.Context(), `
		UPDATE media_files SET status='processing', error=NULL, progress=NULL
		WHERE id = ANY($1) AND status IN ('ready','failed') AND NOT raw_deleted
		RETURNING id`, ids)
	if err != nil {
		return nil, err
	}
	queued, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	for _, id := range queued {
		if err := jobs.Enqueue(r.Context(), tx, "process", map[string]string{"mediaId": id}); err != nil {
			return nil, err
		}
	}
	return queued, tx.Commit(r.Context())
}

// POST /api/media/{id}/cancel stops an upload, download or transcode. Media
// that was never ready is removed with everything stored for it; a
// re-process goes back to ready with the previous output, which the worker
// only replaces at the very end. The worker notices within a few seconds
// (transcode.watchCancel) and stops its job.
func (s *Server) cancelMedia(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx := r.Context()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback(ctx)

	var status, rawKey string
	var hlsKey *string
	err = tx.QueryRow(ctx,
		`SELECT status, raw_key, hls_key FROM media_files WHERE id=$1 FOR UPDATE`, id).
		Scan(&status, &rawKey, &hlsKey)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "media not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	reprocess := status == "processing" && hlsKey != nil
	switch {
	case reprocess:
		_, err = tx.Exec(ctx,
			`UPDATE media_files SET status='ready', progress=NULL, error=NULL WHERE id=$1`, id)
	case status == "uploading" || status == "downloading" || status == "processing":
		_, err = tx.Exec(ctx, `DELETE FROM media_files WHERE id=$1`, id)
	default:
		writeErr(w, http.StatusConflict, "nothing to cancel: the movie isn't uploading, downloading or processing")
		return
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !reprocess {
		s.st.DeleteMedia(ctx, id, rawKey)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteMedia(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	n, err := s.deleteMediaRows(r, []string{id})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n == 0 {
		writeErr(w, http.StatusNotFound, "media not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deleteMediaRows deletes the rows and everything stored for them.
func (s *Server) deleteMediaRows(r *http.Request, ids []string) (int, error) {
	rows, err := s.db.Query(r.Context(),
		`DELETE FROM media_files WHERE id = ANY($1) RETURNING id, raw_key`, ids)
	if err != nil {
		return 0, err
	}
	var gone [][2]string
	for rows.Next() {
		var id, rawKey string
		if err := rows.Scan(&id, &rawKey); err != nil {
			rows.Close()
			return 0, err
		}
		gone = append(gone, [2]string{id, rawKey})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, g := range gone {
		s.st.DeleteMedia(r.Context(), g[0], g[1])
	}
	return len(gone), nil
}

// POST /api/media/{id}/delete-original frees the space of a ready movie's
// original upload. It still plays; download and re-process stop working.
func (s *Server) deleteOriginal(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	n, err := s.deleteOriginals(r, []string{id})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n == 0 {
		writeErr(w, http.StatusConflict, "media not found, not ready, or its original is already gone")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deleteOriginals marks ready media's originals deleted, then removes them
// from S3. Marking first means a failed S3 delete leaves an orphan object
// rather than a row pointing at a missing file.
func (s *Server) deleteOriginals(r *http.Request, ids []string) (int, error) {
	rows, err := s.db.Query(r.Context(), `
		UPDATE media_files SET raw_deleted=true, raw_bytes=0
		WHERE id = ANY($1) AND status='ready' AND NOT raw_deleted
		RETURNING raw_key`, ids)
	if err != nil {
		return 0, err
	}
	keys, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return 0, err
	}
	for _, k := range keys {
		if err := s.st.DeleteKey(r.Context(), k); err != nil {
			log.Printf("delete original %s: %v", k, err)
		}
	}
	return len(keys), nil
}

// POST /api/media/bulk {ids, action, folderId?}: one action on many media.
// action: move (folderId, null = top level) | delete | reprocess | delete_original.
// Media the action doesn't apply to (e.g. not ready) are skipped.
func (s *Server) bulkMedia(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs      []string `json:"ids"`
		Action   string   `json:"action"`
		FolderID *string  `json:"folderId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if len(in.IDs) == 0 || len(in.IDs) > 1000 {
		writeErr(w, http.StatusBadRequest, "ids must list 1-1000 media")
		return
	}
	var done int
	var err error
	switch in.Action {
	case "move":
		var tag pgconn.CommandTag
		tag, err = s.db.Exec(r.Context(),
			`UPDATE media_files SET folder_id=$2 WHERE id = ANY($1)`, in.IDs, emptyToNil(in.FolderID))
		if isForeignKeyViolation(err) {
			writeErr(w, http.StatusBadRequest, "folder not found")
			return
		}
		done = int(tag.RowsAffected())
	case "delete":
		done, err = s.deleteMediaRows(r, in.IDs)
	case "reprocess":
		var ids []string
		ids, err = s.reprocess(r, in.IDs)
		done = len(ids)
	case "delete_original":
		done, err = s.deleteOriginals(r, in.IDs)
	default:
		writeErr(w, http.StatusBadRequest, "action must be move, delete, reprocess or delete_original")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"done": done, "skipped": len(in.IDs) - done})
}
