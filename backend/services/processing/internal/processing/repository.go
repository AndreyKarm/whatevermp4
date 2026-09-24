package processing

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

type Job struct {
	ID        string  `json:"id"`
	VersionID string  `json:"versionId"`
	Stage     string  `json:"stage"`
	Progress  int16   `json:"progress"`
	Error     *string `json:"error,omitempty"`
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CreateJob(ctx context.Context, versionID string) (*Job, error) {
	j := &Job{VersionID: versionID, Stage: "queued"}
	err := r.db.QueryRow(ctx,
		`insert into processing_jobs (version_id) values ($1) returning id`,
		versionID,
	).Scan(&j.ID)
	if err != nil {
		return nil, err
	}
	return j, nil
}

func (r *Repository) GetJob(ctx context.Context, id string) (*Job, error) {
	var j Job
	err := r.db.QueryRow(ctx,
		`select id, version_id, stage, progress, error from processing_jobs where id = $1`, id,
	).Scan(&j.ID, &j.VersionID, &j.Stage, &j.Progress, &j.Error)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &j, nil
}

func (r *Repository) UpdateStage(ctx context.Context, id, stage string, progress int16, jobErr *string) error {
	_, err := r.db.Exec(ctx,
		`update processing_jobs set stage = $1, progress = $2, error = $3, updated_at = now() where id = $4`,
		stage, progress, jobErr, id,
	)
	return err
}

// NextQueuedJob picks up the oldest queued job for a worker to process.
func (r *Repository) NextQueuedJob(ctx context.Context) (*Job, error) {
	var j Job
	err := r.db.QueryRow(ctx,
		`select id, version_id, stage, progress, error from processing_jobs
		 where stage = 'queued' order by created_at asc limit 1`,
	).Scan(&j.ID, &j.VersionID, &j.Stage, &j.Progress, &j.Error)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &j, nil
}
