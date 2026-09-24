package catalog

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) ListTitles(ctx context.Context) ([]Title, error) {
	rows, err := r.db.Query(ctx,
		`select id, type, name, year, description, poster_key, created_at
		 from titles order by created_at desc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Title
	for rows.Next() {
		var t Title
		if err := rows.Scan(&t.ID, &t.Type, &t.Name, &t.Year, &t.Description, &t.PosterKey, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *Repository) GetTitle(ctx context.Context, id string) (*Title, error) {
	var t Title
	err := r.db.QueryRow(ctx,
		`select id, type, name, year, description, poster_key, created_at
		 from titles where id = $1`, id,
	).Scan(&t.ID, &t.Type, &t.Name, &t.Year, &t.Description, &t.PosterKey, &t.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &t, nil
}

func (r *Repository) CreateTitle(ctx context.Context, titleType, name string, year *int, description *string, createdBy string) (*Title, error) {
	t := &Title{Type: titleType, Name: name, Year: year, Description: description}
	err := r.db.QueryRow(ctx,
		`insert into titles (type, name, year, description, created_by)
		 values ($1, $2, $3, $4, $5)
		 returning id, created_at`,
		titleType, name, year, description, createdBy,
	).Scan(&t.ID, &t.CreatedAt)
	if err != nil {
		return nil, err
	}
	return t, nil
}

// CreateMediaVersion is called by the gateway/upload flow to register a new
// version before handing it off to the Processing service.
func (r *Repository) CreateMediaVersion(ctx context.Context, titleID, episodeID *string, sourceType, originalKey string) (*MediaVersion, error) {
	mv := &MediaVersion{TitleID: titleID, EpisodeID: episodeID, Label: "default", Status: "queued"}
	err := r.db.QueryRow(ctx,
		`insert into media_versions (title_id, episode_id, source_type, original_key)
		 values ($1, $2, $3, $4)
		 returning id`,
		titleID, episodeID, sourceType, originalKey,
	).Scan(&mv.ID)
	if err != nil {
		return nil, err
	}
	return mv, nil
}

// UpdateMediaVersionStatus is called by the Processing service as a job
// progresses (queued -> downloading -> transcoding -> packaging -> done/failed).
func (r *Repository) UpdateMediaVersionStatus(ctx context.Context, versionID, status string, masterManifestKey *string) error {
	_, err := r.db.Exec(ctx,
		`update media_versions set status = $1, master_manifest_key = coalesce($2, master_manifest_key), updated_at = now()
		 where id = $3`,
		status, masterManifestKey, versionID,
	)
	return err
}
