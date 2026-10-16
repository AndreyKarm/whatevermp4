package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"liotom/whateverserver/internal/storage"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

const playbackTTL = 12 * time.Hour

// signPlayback creates a URL-safe HMAC token bound to a media ID and expiration.
func (s *Server) signPlayback(mediaID string, exp time.Time) string {
	u := exp.Unix()
	mac := hmac.New(sha256.New, []byte(s.cfg.PlaybackSecret))
	fmt.Fprintf(mac, "%s:%d", mediaID, u)
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("%d.%s", u, sig)
}

func (s *Server) verifyPlayback(token, mediaID string) bool {
	expStr, _, ok := strings.Cut(token, ".")
	if !ok {
		return false
	}
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return false
	}
	expected := s.signPlayback(mediaID, time.Unix(exp, 0))
	return hmac.Equal([]byte(token), []byte(expected))
}

// playbackBase returns "/play/{token}/{id}" so the frontend can append
// "/master.m3u8", "/poster.jpg", "/subs/...", or "/fonts/...".
func (s *Server) playbackBase(mediaID string) string {
	tok := s.signPlayback(mediaID, time.Now().Add(playbackTTL))
	return fmt.Sprintf("/play/%s/%s", tok, mediaID)
}

// GET /play/{token}/{id}/*
func (s *Server) handlePlayback(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	id := chi.URLParam(r, "id")
	rest := chi.URLParam(r, "*")

	if id == "" || rest == "" || strings.Contains(rest, "..") {
		http.NotFound(w, r)
		return
	}
	if !s.verifyPlayback(token, id) {
		http.Error(w, "invalid or expired playback token", http.StatusForbidden)
		return
	}

	key := "hls/" + id + "/" + rest
	out, err := s.st.Get(r.Context(), key, r.Header.Get("Range"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer out.Body.Close()

	w.Header().Set("Content-Type", storage.ContentType(rest))
	w.Header().Set("Accept-Ranges", "bytes")
	if strings.HasSuffix(rest, ".m3u8") {
		w.Header().Set("Cache-Control", "private, max-age=300")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	if out.ContentLength != nil {
		w.Header().Set("Content-Length", strconv.FormatInt(*out.ContentLength, 10))
	}
	if out.ContentRange != nil {
		w.Header().Set("Content-Range", *out.ContentRange)
		w.WriteHeader(http.StatusPartialContent)
	}
	_, _ = io.Copy(w, out.Body)
}
