package auth

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func InitDB(dbURL string) (*pgxpool.Pool, error) {
	return pgxpool.New(context.Background(), dbURL)
}
