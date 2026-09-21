package auth

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// InitDB opens a connection pool.
func InitDB(dbpath string) (*pgxpool.Pool, error) {
	return pgxpool.New(context.Background(), dbpath)
}
