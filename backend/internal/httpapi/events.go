package httpapi

import (
	"context"
	"liotom/whateverserver/internal/rooms"
	"net/http"
	"time"

	"github.com/coder/websocket"
)

// GET /api/events (WebSocket): forwards every change event (JSON, see
// internal/events) so pages update instead of polling.
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
		case payload, ok := <-ch:
			if !ok {
				// fell behind: the client reconnects and resyncs
				conn.Close(websocket.StatusTryAgainLater, "too many events")
				return
			}
			wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := conn.Write(wctx, websocket.MessageText, []byte(payload))
			cancel()
			if err != nil {
				return
			}
		}
	}
}
