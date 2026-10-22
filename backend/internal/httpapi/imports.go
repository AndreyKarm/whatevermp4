package httpapi

import (
	"encoding/json"
	"liotom/whateverserver/internal/jobs"
	"liotom/whateverserver/internal/storage"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/google/uuid"
)

// POST /api/imports {url, name?, referer?}: download an HLS (.m3u8) stream.
func (s *Server) createImport(w http.ResponseWriter, r *http.Request) {
	var in struct {
		URL     string `json:"url"`
		Name    string `json:"name"`
		Referer string `json:"referer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	u, ok := httpURL(in.URL)
	if !ok {
		writeErr(w, http.StatusBadRequest, "url must be an http(s) link")
		return
	}
	referer := strings.TrimSpace(in.Referer)
	if referer != "" {
		if _, ok := httpURL(referer); !ok {
			writeErr(w, http.StatusBadRequest, "referer must be an http(s) link")
			return
		}
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = importName(u)
	}

	id := uuid.NewString()
	key := "raw/" + id + "/" + storage.SafeName(name) + ".mkv"
	season, ep := storage.ParseSE(name)
	_, err := s.db.Exec(r.Context(), `
		INSERT INTO media_files (id, uploaded_by, filename, status, raw_key, season, episode)
		VALUES ($1, $2, $3, 'downloading', $4, $5, $6)`,
		id, nickname(r), name, key, season, ep)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := jobs.Enqueue(r.Context(), s.db, "import", map[string]string{
		"mediaId": id, "url": u.String(), "referer": referer,
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func httpURL(s string) (*url.URL, bool) {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, false
	}
	return u, true
}

// importName derives a display name from the playlist URL when none is given.
func importName(u *url.URL) string {
	base := strings.TrimSuffix(path.Base(u.Path), ".m3u8")
	// e.g. "0m5jt.mp4:hls:manifest" → "0m5jt"
	base, _, _ = strings.Cut(base, ":")
	base = strings.TrimSuffix(base, path.Ext(base))
	switch strings.ToLower(base) {
	case "", ".", "/", "index", "master", "playlist", "manifest", "prog_index":
		// generic playlist name: use the parent directory, or the host
		if dir := path.Base(path.Dir(u.Path)); dir != "." && dir != "/" {
			return dir
		}
		return u.Host
	}
	return base
}
