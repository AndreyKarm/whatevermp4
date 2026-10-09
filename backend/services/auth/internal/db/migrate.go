package db

import (
	"context"
	"embed"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/migrate"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func RunMigrations(ctx context.Context, db *bun.DB) error {
	migrations := migrate.NewMigrations()
	if err := migrations.Discover(migrationsFS); err != nil {
		return err
	}

	m := migrate.NewMigrator(db, migrations)
	if err := m.Init(ctx); err != nil {
		return err
	}
	if _, err := m.Migrate(ctx); err != nil {
		return err
	}
	return nil
}
