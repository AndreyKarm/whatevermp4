package catalog

import (
	"context"
	"database/sql"
	"errors"

	"github.com/uptrace/bun"
)

var ErrNotFound = errors.New("not found")

type Repository struct {
	db *bun.DB
}

func NewRepository(db *bun.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) ListTitles(ctx context.Context) ([]Title, error) {
	titles := []Title{} // empty slice, so JSON is [] rather than null
	err := r.db.NewSelect().
		Model(&titles).
		Order("created_at DESC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return titles, nil
}

func (r *Repository) GetTitle(ctx context.Context, id string) (*Title, error) {
	t := new(Title)
	err := r.db.NewSelect().
		Model(t).
		Where("id = ?", id).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return t, nil
}

func (r *Repository) CreateTitle(ctx context.Context, titleType, name string, year *int, description *string, createdBy string) (*Title, error) {
	t := &Title{
		Type:        titleType,
		Name:        name,
		Year:        year,
		Description: description,
		CreatedBy:   &createdBy,
	}
	_, err := r.db.NewInsert().
		Model(t).
		Returning("id, created_at, updated_at").
		Exec(ctx)
	if err != nil {
		return nil, err
	}
	return t, nil
}

// CreateMediaVersion is called by the gateway/upload flow to register a new
// version before handing it off to the Processing service.
func (r *Repository) CreateMediaVersion(ctx context.Context, titleID, episodeID *string, sourceType, originalKey string) (*MediaVersion, error) {
	mv := &MediaVersion{
		TitleID:     titleID,
		EpisodeID:   episodeID,
		SourceType:  sourceType,
		OriginalKey: &originalKey,
	}
	// label/status come from the column defaults ('default' / 'queued').
	_, err := r.db.NewInsert().
		Model(mv).
		Returning("id, label, status, created_at, updated_at").
		Exec(ctx)
	if err != nil {
		return nil, err
	}
	return mv, nil
}

// UpdateMediaVersionStatus is called by the Processing service as a job
// progresses (queued -> downloading -> transcoding -> packaging -> done/failed).
func (r *Repository) UpdateMediaVersionStatus(ctx context.Context, versionID, status string, masterManifestKey *string) error {
	_, err := r.db.NewUpdate().
		Model((*MediaVersion)(nil)).
		Set("status = ?", status).
		Set("master_manifest_key = COALESCE(?, master_manifest_key)", masterManifestKey).
		Set("updated_at = now()").
		Where("id = ?", versionID).
		Exec(ctx)
	return err
}
