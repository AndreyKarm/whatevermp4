package rooms

type State struct {
	MediaID   string   `json:"mediaId"`
	Playing   bool     `json:"playing"`
	Position  float64  `json:"position"`
	UpdatedAt int64    `json:"updatedAt"` // unix ms, server clock
	Rate      float64  `json:"rate"`
	Queue     []string `json:"queue,omitempty"` // media IDs that play next, in order
}

type Member struct {
	Name      string `json:"name"`
	Buffering bool   `json:"buffering"`
	// From the member's last status: seconds ahead (+) or behind (-) the
	// room clock, and seconds buffered ahead of its playhead.
	Drift *float64 `json:"drift,omitempty"`
	Ahead *float64 `json:"ahead,omitempty"`
	// round trip to the server in ms, as the member's last ping measured it
	Ping *int `json:"ping,omitempty"`
}

// In: play | pause | seek | rate | change_media | queue | next | buffering | status | chat | ping
type In struct {
	Type      string   `json:"type"`
	MediaID   string   `json:"mediaId,omitempty"`
	Position  float64  `json:"position,omitempty"` // play, seek, change_media (start point)
	Rate      float64  `json:"rate,omitempty"`
	Queue     []string `json:"queue,omitempty"` // queue: the whole new list
	Text      string   `json:"text,omitempty"`
	Buffering bool     `json:"buffering,omitempty"`
	Drift     float64  `json:"drift,omitempty"` // status
	Ahead     float64  `json:"ahead,omitempty"` // status
	T         int64    `json:"t,omitempty"`
	RTT       float64  `json:"rtt,omitempty"` // ping: the previous ping's round trip in ms
}

// RoomInfo is the room's settings and who controls it, sent on join and
// whenever the owner changes them.
type RoomInfo struct {
	Name        string   `json:"name"`
	Owner       string   `json:"owner"`
	Everyone    bool     `json:"everyone"`
	Controllers []string `json:"controllers"`
	Private     bool     `json:"private"`
	HasPassword bool     `json:"hasPassword"`
	InviteCode  string   `json:"inviteCode,omitempty"` // only to the owner of a private room
}

// Out: state | queue | room | members | chat | pong | error
//   - state: playback (and the queue) changed
//   - queue: only the queue changed
//   - room: settings, owner or controllers changed
type Out struct {
	Type    string    `json:"type"`
	State   *State    `json:"state,omitempty"`
	Holding bool      `json:"holding,omitempty"` // paused while waiting for buffering
	Queue   []string  `json:"queue,omitempty"`
	Room    *RoomInfo `json:"room,omitempty"`
	Members []Member  `json:"members,omitempty"`
	From    *Member   `json:"from,omitempty"`
	Text    string    `json:"text,omitempty"`
	T       int64     `json:"t,omitempty"`
	Server  int64     `json:"server,omitempty"`
}
