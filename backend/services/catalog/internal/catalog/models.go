package catalog

import "time"

type Title struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	Name        string    `json:"name"`
	Year        *int      `json:"year,omitempty"`
	Description *string   `json:"description,omitempty"`
	PosterKey   *string   `json:"posterKey,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}

type MediaVersion struct {
	ID        string  `json:"id"`
	TitleID   *string `json:"titleId,omitempty"`
	EpisodeID *string `json:"episodeId,omitempty"`
	Label     string  `json:"label"`
	Status    string  `json:"status"`
}
