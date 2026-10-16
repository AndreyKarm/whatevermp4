package jobs

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Job struct {
	ID       int64
	Type     string
	Payload  json.RawMessage
	Attempts int
}

func Enqueue(ctx context.Context, db *pgxpool.Pool, typ string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, `INSERT INTO jobs (type, payload) VALUES ($1, $2)`, typ, b)
	return err
}

// Claim returns (nil, nil) when the queue is empty.
func Claim(ctx context.Context, db *pgxpool.Pool, types []string) (*Job, error) {
	var j Job
	err := db.QueryRow(ctx, `
		UPDATE jobs SET status='running', attempts=attempts+1, locked_at=now()
		WHERE id = (
			SELECT id FROM jobs
			WHERE status='queued' AND run_at <= now() AND type = ANY($1)
			ORDER BY id FOR UPDATE SKIP LOCKED LIMIT 1)
		RETURNING id, type, payload, attempts`, types).
		Scan(&j.ID, &j.Type, &j.Payload, &j.Attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &j, err
}

func Done(ctx context.Context, db *pgxpool.Pool, id int64) error {
	_, err := db.Exec(ctx, `UPDATE jobs SET status='done' WHERE id=$1`, id)
	return err
}

// Fail requeues with a delay, or marks dead when permanent.
func Fail(ctx context.Context, db *pgxpool.Pool, id int64, msg string, permanent bool) error {
	status := "queued"
	if permanent {
		status = "failed"
	}
	_, err := db.Exec(ctx, `
		UPDATE jobs SET status=$2, last_error=$3, run_at=now()+interval '30 seconds'
		WHERE id=$1`, id, status, msg)
	return err
}
