// Package testdb gives tests a throwaway, fully migrated Postgres database.
//
// Set TEST_DATABASE_URL to any database on a server where that user may
// CREATE DATABASE (the compose db works:
// postgres://movieuser:…@localhost:5432/local?sslmode=disable). Tests that
// need a database are skipped when it is unset.
package testdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"liotom/whateverserver/internal/migrate"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// New creates an empty database, applies the migrations and drops it when
// the test ends.
func New(t testing.TB) *pgxpool.Pool {
	t.Helper()
	pool := Empty(t)
	if err := migrate.Up(context.Background(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return pool
}

// Empty creates a database without running the migrations.
func Empty(t testing.TB) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	name := "test_" + hex.EncodeToString(b)
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		admin.Close(ctx)
		t.Fatalf("create database: %v", err)
	}

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.Database = name
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)"); err != nil {
			t.Logf("drop database %s: %v", name, err)
		}
		admin.Close(ctx)
	})
	return pool
}
