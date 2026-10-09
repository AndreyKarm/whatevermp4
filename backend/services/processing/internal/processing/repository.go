package processing

import (
	"context"
	"database/sql"
	"errors"

	"github.com/uptrace/bun"
)

var ErrNotFound = errors.New("not found")

type Job struct {
	bun.BaseModel `bun:"table:processing_jobs,alias:j" json:"-"`

	ID        string  `bun:"id,pk,type:uuid,nullzero,default:gen_random_uuid()" json:"id"`
	VersionID string  `bun:"version_id,type:uuid,notnull" json:"versionId"`
	Stage     string  `bun:"stage,notnull" json:"stage"`
	Progress  int16   `bun:"progress,notnull" json:"progress"`
	Error     *string `bun:"error" json:"error,omitempty"`
}

type Repository struct {
	db *bun.DB
}

func NewRepository(db *bun.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CreateJob(ctx context.Context, versionID string) (*Job, error) {
	j := &Job{VersionID: versionID, Stage: "queued"}
	if _, err := r.db.NewInsert().Model(j).Returning("id").Exec(ctx); err != nil {
		return nil, err
	}
	return j, nil
}

func (r *Repository) GetJob(ctx context.Context, id string) (*Job, error) {
	j := new(Job)
	err := r.db.NewSelect().Model(j).Where("j.id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return j, nil
}

func (r *Repository) UpdateStage(ctx context.Context, id, stage string, progress int16, jobErr *string) error {
	_, err := r.db.NewUpdate().
		Model((*Job)(nil)).
		Set("stage = ?", stage).
		Set("progress = ?", progress).
		Set("error = ?", jobErr).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

// NextQueuedJob picks up the oldest queued job for a worker to process.
func (r *Repository) NextQueuedJob(ctx context.Context) (*Job, error) {
	j := new(Job)
	err := r.db.NewSelect().
		Model(j).
		Where("j.stage = ?", "queued").
		OrderExpr("j.created_at ASC").
		Limit(1).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return j, nil
}
