package rooms

import (
	"context"
	"liotom/whateverserver/internal/auth"
	"net/http"
	"net/url"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"
)

// Browsers can't set headers on WebSocket, so the JWT comes in ?token=.
func Handler(h *Hub, v *auth.Verifier, origins []string) http.HandlerFunc {
	var patterns []string
	for _, o := range origins {
		if u, err := url.Parse(o); err == nil && u.Host != "" {
			patterns = append(patterns, u.Host)
		}
	}
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := v.Verify(r.Context(), r.URL.Query().Get("token"))
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: patterns})
		if err != nil {
			return
		}
		defer conn.CloseNow()

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()

		c := &Client{User: user, conn: conn, send: make(chan []byte, 64)}
		go c.writeLoop(ctx)
		if err := h.Join(ctx, chi.URLParam(r, "id"), c); err != nil {
			conn.Close(websocket.StatusPolicyViolation, "room not found")
			return
		}
		c.readLoop(ctx)
		h.Leave(c)
	}
}
