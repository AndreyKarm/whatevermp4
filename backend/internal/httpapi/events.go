package httpapi

import (
	"context"
	"encoding/json"
	"liotom/whateverserver/internal/rooms"
	"net/http"
	"time"

	"github.com/coder/websocket"
)

// GET /api/events (WebSocket): sends {"type": "<topic>"} whenever media,
// torrents or rooms change, so pages can refetch instead of polling.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: rooms.OriginPatterns(s.cfg.AllowedOrigins),
	})
	if err != nil {
		return
	}
	defer conn.CloseNow()

	// We never read from the client; CloseRead handles pings/close frames
	// and cancels ctx when the client goes away.
	ctx := conn.CloseRead(r.Context())
	ch, unsubscribe := s.bus.Subscribe()
	defer unsubscribe()

	for {
		select {
		case <-ctx.Done():
			return
		case topic := <-ch:
			b, _ := json.Marshal(map[string]string{"type": topic})
			wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := conn.Write(wctx, websocket.MessageText, b)
			cancel()
			if err != nil {
				return
			}
		}
	}
}
