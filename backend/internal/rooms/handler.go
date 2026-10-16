package rooms

import (
	"context"
	"liotom/whateverserver/internal/auth"
	"net/http"
	"net/url"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"
)

func Handler(h *Hub, v *auth.Verifier, origins []string) http.HandlerFunc {
	var patterns []string
	for _, o := range origins {
		if u, err := url.Parse(o); err == nil && u.Host != "" {
			patterns = append(patterns, u.Host)
		}
	}
	return func(w http.ResponseWriter, r *http.Request) {
		tok := auth.ExtractToken(
			r.Header.Get("Authorization"),
			r.URL.Query().Get("token"),
			func(name string) string {
				if c, err := r.Cookie(name); err == nil {
					return c.Value
				}
				return ""
			},
		)
		user, err := v.Verify(r.Context(), tok)
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
