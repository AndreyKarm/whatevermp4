package rooms

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"
)

const maxNickname = 32

// CleanNickname trims and length-limits a self-chosen nickname.
func CleanNickname(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "Anonymous"
	}
	if utf8.RuneCountInString(s) > maxNickname {
		s = string([]rune(s)[:maxNickname])
	}
	return s
}

// OriginPatterns turns ALLOWED_ORIGINS URLs into websocket.AcceptOptions host patterns.
func OriginPatterns(origins []string) []string {
	var patterns []string
	for _, o := range origins {
		if u, err := url.Parse(o); err == nil && u.Host != "" {
			patterns = append(patterns, u.Host)
		}
	}
	return patterns
}

func Handler(h *Hub, origins []string) http.HandlerFunc {
	patterns := OriginPatterns(origins)
	return func(w http.ResponseWriter, r *http.Request) {
		name := CleanNickname(r.URL.Query().Get("nickname"))

		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: patterns})
		if err != nil {
			return
		}
		defer conn.CloseNow()

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()

		c := &Client{Nickname: name, conn: conn, send: make(chan []byte, 64)}
		go c.writeLoop(ctx)
		if err := h.Join(ctx, chi.URLParam(r, "id"), c); err != nil {
			conn.Close(websocket.StatusPolicyViolation, "room not found")
			return
		}
		c.readLoop(ctx)
		h.Leave(c)
	}
}
