package catalog

import (
	"time"

	"github.com/uptrace/bun"
)

type Title struct {
	bun.BaseModel `bun:"table:titles,alias:t" json:"-"`

	ID          string    `bun:"id,pk,type:uuid,nullzero" json:"id"`
	Type        string    `bun:"type,notnull" json:"type"`
	Name        string    `bun:"name,notnull" json:"name"`
	Year        *int      `bun:"year" json:"year,omitempty"`
	Description *string   `bun:"description" json:"description,omitempty"`
	PosterKey   *string   `bun:"poster_key" json:"posterKey,omitempty"`
	CreatedBy   *string   `bun:"created_by,type:uuid" json:"-"`
	CreatedAt   time.Time `bun:"created_at,notnull,nullzero" json:"createdAt"`
	UpdatedAt   time.Time `bun:"updated_at,notnull,nullzero" json:"-"`
}

type MediaVersion struct {
	bun.BaseModel `bun:"table:media_versions,alias:mv" json:"-"`

	ID                string  `bun:"id,pk,type:uuid,nullzero" json:"id"`
	TitleID           *string `bun:"title_id,type:uuid" json:"titleId,omitempty"`
	EpisodeID         *string `bun:"episode_id,type:uuid" json:"episodeId,omitempty"`
	Label             string  `bun:"label,notnull,nullzero" json:"label"`
	SourceType        string  `bun:"source_type,notnull" json:"-"`
	OriginalKey       *string `bun:"original_key" json:"-"`
	MasterManifestKey *string `bun:"master_manifest_key" json:"-"`
	Status            string  `bun:"status,notnull,nullzero" json:"status"`

	CreatedAt time.Time `bun:"created_at,notnull,nullzero" json:"-"`
	UpdatedAt time.Time `bun:"updated_at,notnull,nullzero" json:"-"`
}
