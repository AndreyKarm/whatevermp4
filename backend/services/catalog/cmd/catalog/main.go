package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"movie-platform/services/catalog/internal/catalog"
	"movie-platform/services/catalog/internal/db"
	"movie-platform/shared/authmw"
	"movie-platform/shared/config"
)

func main() {
	config.LoadEnv()

	dbURL := os.Getenv("CATALOG_DATABASE_URL")
	if err := db.RunMigrations(dbURL); err != nil {
		log.Fatalf("migrations failed: %v", err)
	}

	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	jwks := authmw.NewJWKSClient(os.Getenv("AUTH_SERVICE_URL") + "/auth/.well-known/jwks.json")
	internalSecret := []byte(os.Getenv("INTERNAL_SERVICE_SECRET"))
	repo := catalog.NewRepository(pool)
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
