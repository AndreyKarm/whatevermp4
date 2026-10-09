package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"

	"movie-platform/services/processing/internal/db"
	"movie-platform/services/processing/internal/processing"
	"movie-platform/shared/authmw"
	"movie-platform/shared/config"
	"movie-platform/shared/httpx"
)

func main() {
	config.LoadEnv()

	ctx := context.Background()

	dbURL := os.Getenv("PROCESSING_DATABASE_URL")
	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dbURL)))
	bunDB := bun.NewDB(sqldb, pgdialect.New())
	defer bunDB.Close()

	if err := db.RunMigrations(ctx, bunDB); err != nil {
		log.Fatalf("migrations failed: %v", err)
	}

	jwks := authmw.NewJWKSClient(os.Getenv("AUTH_SERVICE_URL") + "/auth/.well-known/jwks.json")
	internalSecret := []byte(os.Getenv("INTERNAL_SERVICE_SECRET"))
	repo := processing.NewRepository(bunDB)

	worker := processing.NewWorker(repo, internalSecret)
	go worker.Run(ctx)

	r := chi.NewRouter()

	r.Group(func(r chi.Router) {
		r.Use(authmw.RequireAuth(jwks))
		r.Use(authmw.RequireRole("admin"))

		r.Post("/processing/jobs", func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				VersionID string `json:"versionId"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.VersionID == "" {
				httpx.Error(w, http.StatusBadRequest, "invalid body")
				return
			}
			job, err := repo.CreateJob(r.Context(), body.VersionID)
			if err != nil {
				log.Printf("create job: %v", err)
				httpx.Error(w, http.StatusInternalServerError, "failed to create job")
				return
			}
			httpx.WriteJSON(w, http.StatusCreated, job)
		})

		r.Get("/processing/jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
			id := chi.URLParam(r, "id")
			job, err := repo.GetJob(r.Context(), id)
			if err != nil {
				httpx.Error(w, http.StatusNotFound, "job not found")
				return
			}
			httpx.WriteJSON(w, http.StatusOK, job)
		})
	})

	port := os.Getenv("PROCESSING_PORT")
	log.Printf("processing service listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}
