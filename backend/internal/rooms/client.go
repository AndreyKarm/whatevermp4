package rooms

import (
	"context"
	"encoding/json"
	"time"

	"github.com/coder/websocket"
)

type Client struct {
	Nickname  string
	conn      *websocket.Conn
	send      chan []byte
	room      *Room
	buffering bool // guarded by room.mu
}

func (c *Client) trySend(b []byte) {
	select {
	case c.send <- b:
	default: // too slow, drop the connection
		go c.conn.CloseNow()
	}
}

func (c *Client) sendOut(o Out) {
	b, _ := json.Marshal(o)
	c.trySend(b)
}

func (c *Client) writeLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case b := <-c.send:
			wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := c.conn.Write(wctx, websocket.MessageText, b)
			cancel()
			if err != nil {
				return
			}
		}
	}
}

func (c *Client) readLoop(ctx context.Context) {
	c.conn.SetReadLimit(8 << 10)
	for {
		_, data, err := c.conn.Read(ctx)
		if err != nil {
			return
		}
		var in In
		if json.Unmarshal(data, &in) != nil {
			continue
		}
		if in.Type == "ping" {
			c.sendOut(Out{Type: "pong", T: in.T, Server: nowMs()})
			continue
		}
		c.room.handle(c, in)
	}
}
