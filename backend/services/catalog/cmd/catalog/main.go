package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"

	"movie-platform/services/catalog/internal/catalog"
	"movie-platform/services/catalog/internal/db"
	"movie-platform/shared/authmw"
	"movie-platform/shared/config"
)

func main() {
	config.LoadEnv()
	ctx := context.Background()

	dbURL := os.Getenv("CATALOG_DATABASE_URL")

	sqldb, err := sql.Open("pgx", dbURL)
	if err != nil {
		log.Fatal(err)
	}
	bunDB := bun.NewDB(sqldb, pgdialect.New())
	defer bunDB.Close()

	if err := bunDB.PingContext(ctx); err != nil {
		log.Fatalf("db ping failed: %v", err)
	}
	if err := db.RunMigrations(ctx, bunDB); err != nil {
		log.Fatalf("migrations failed: %v", err)
	}

	jwks := authmw.NewJWKSClient(os.Getenv("AUTH_SERVICE_URL") + "/auth/.well-known/jwks.json")
	internalSecret := []byte(os.Getenv("INTERNAL_SERVICE_SECRET"))
	repo := catalog.NewRepository(bunDB)
	h := catalog.NewHandler(repo)

	r := chi.NewRouter()

	r.Get("/catalog/titles", h.ListTitles)
	r.Get("/catalog/titles/{id}", h.GetTitle)

	r.Group(func(r chi.Router) {
		r.Use(authmw.RequireAuth(jwks))
		r.Use(authmw.RequireRole("admin"))
		r.Post("/catalog/titles", h.CreateTitle)
	})

	r.Group(func(r chi.Router) {
		// Only Processing may call this — never an end-user, even an admin.
		r.Use(authmw.RequireServiceAuth(internalSecret, "processing"))
		r.Patch("/catalog/versions/{id}/status", h.UpdateMediaVersionStatus)
	})

	port := os.Getenv("CATALOG_PORT")
	log.Printf("catalog service listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}
