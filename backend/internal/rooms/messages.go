package rooms

type State struct {
	MediaID   string  `json:"mediaId"`
	Playing   bool    `json:"playing"`
	Position  float64 `json:"position"`
	UpdatedAt int64   `json:"updatedAt"` // unix ms, server clock
	Rate      float64 `json:"rate"`
}

type Member struct {
	Name      string `json:"name"`
	Buffering bool   `json:"buffering"`
}

// In: play | pause | seek | rate | change_media | buffering | chat | ping
type In struct {
	Type      string  `json:"type"`
	MediaID   string  `json:"mediaId,omitempty"`
	Position  float64 `json:"position,omitempty"` // play, seek, change_media (start point)
	Rate      float64 `json:"rate,omitempty"`
	Text      string  `json:"text,omitempty"`
	Buffering bool    `json:"buffering,omitempty"`
	T         int64   `json:"t,omitempty"`
}

// Out: state | members | chat | pong | error
type Out struct {
	Type     string   `json:"type"`
	State    *State   `json:"state,omitempty"`
	Holding  bool     `json:"holding,omitempty"` // paused while waiting for buffering
	Members  []Member `json:"members,omitempty"`
	From     *Member  `json:"from,omitempty"`
	Text     string   `json:"text,omitempty"`
	T        int64    `json:"t,omitempty"`
	Server   int64    `json:"server,omitempty"`
	Owner    string   `json:"owner,omitempty"`
	Everyone bool     `json:"everyone,omitempty"`
}
