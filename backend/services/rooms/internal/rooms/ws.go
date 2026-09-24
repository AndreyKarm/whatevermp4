package rooms

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"

	"movie-platform/shared/authmw"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// In production, restrict this to your actual frontend origin(s)
	// instead of allowing any origin.
	CheckOrigin: func(r *http.Request) bool { return true },
}

// ClientMessage is sent from a browser client over the socket whenever it
// changes playback state (play, pause, or seek).
type ClientMessage struct {
	Type      string  `json:"type"` // "play" | "pause" | "seek"
	Position  float64 `json:"position"`
	IsPlaying bool    `json:"isPlaying"`
}

// BroadcastMessage is relayed from the server to every other member of a
// room whenever any member changes playback state.
type BroadcastMessage struct {
	Type      string  `json:"type"`
	Position  float64 `json:"position"`
	IsPlaying bool    `json:"isPlaying"`
	UserID    string  `json:"userId"`
	At        int64   `json:"at"` // server unix millis, for client drift correction
}

type client struct {
	conn   *websocket.Conn
	userID string
	send   chan BroadcastMessage
}

// Hub tracks connected clients per room and fans out playback updates.
type Hub struct {
	repo *Repository

	mu    sync.Mutex
	rooms map[string]map[*client]bool // roomID -> connected clients
}

func NewHub(repo *Repository) *Hub {
	return &Hub{repo: repo, rooms: make(map[string]map[*client]bool)}
}

func (h *Hub) join(roomID string, c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.rooms[roomID] == nil {
		h.rooms[roomID] = make(map[*client]bool)
	}
	h.rooms[roomID][c] = true
}

func (h *Hub) leave(roomID string, c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.rooms[roomID][c]; ok {
		delete(h.rooms[roomID], c)
		close(c.send)
	}
	if len(h.rooms[roomID]) == 0 {
		delete(h.rooms, roomID)
	}
}

func (h *Hub) broadcast(roomID string, msg BroadcastMessage, except *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.rooms[roomID] {
		if c == except {
			continue
		}
		select {
		case c.send <- msg:
		default:
			// Slow/stuck client: drop the message rather than block the hub.
		}
	}
}

// ServeWS upgrades the connection and pumps messages for a single client.
//
// Auth note: browsers cannot set custom headers on a WebSocket handshake,
// so the access token is passed as a query parameter (?token=...) instead
// of an Authorization header. This route is mounted outside the normal
// authmw.RequireAuth middleware chain and verifies the token manually.
func (h *Hub) ServeWS(jwks *authmw.JWKSClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		roomCode := chi.URLParam(r, "code")

		token := r.URL.Query().Get("token")
		if token == "" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		claims, err := authmw.ParseAccessToken(jwks, token)
		if err != nil {
			http.Error(w, "invalid or expired token", http.StatusUnauthorized)
			return
		}

		room, err := h.repo.GetRoomByCode(r.Context(), roomCode)
		if err != nil {
			http.Error(w, "room not found", http.StatusNotFound)
			return
		}
		if err := h.repo.AddMember(r.Context(), room.ID, claims.UserID); err != nil {
			http.Error(w, "failed to join room", http.StatusInternalServerError)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("ws upgrade failed: %v", err)
			return
		}

		c := &client{conn: conn, userID: claims.UserID, send: make(chan BroadcastMessage, 16)}
		h.join(room.ID, c)

		// Immediately bring the newly joined client up to speed with
		// whatever state is currently persisted for the room.
		c.send <- BroadcastMessage{
			Type:      "sync",
			Position:  room.PositionSeconds,
			IsPlaying: room.IsPlaying,
			UserID:    room.HostUserID,
			At:        time.Now().UnixMilli(),
		}

		go h.writePump(c)
		h.readPump(r.Context(), room.ID, c)
	}
}

func (h *Hub) readPump(ctx context.Context, roomID string, c *client) {
	defer func() {
		h.leave(roomID, c)
		c.conn.Close()
	}()

	// Keep the connection alive and detect dead peers.
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		var msg ClientMessage
		if err := c.conn.ReadJSON(&msg); err != nil {
			return
		}

		switch msg.Type {
		case "play", "pause", "seek":
		default:
			continue // ignore unknown message types
		}

		if err := h.repo.UpdatePlaybackState(ctx, roomID, msg.Position, msg.IsPlaying); err != nil {
			log.Printf("failed to persist playback state for room %s: %v", roomID, err)
		}

		h.broadcast(roomID, BroadcastMessage{
			Type:      msg.Type,
			Position:  msg.Position,
			IsPlaying: msg.IsPlaying,
			UserID:    c.userID,
			At:        time.Now().UnixMilli(),
		}, c)
	}
}

func (h *Hub) writePump(c *client) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteJSON(msg); err != nil {
				return
			}
		case <-ticker.C:
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
