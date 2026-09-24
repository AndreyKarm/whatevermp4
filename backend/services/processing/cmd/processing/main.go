package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"movie-platform/services/processing/internal/db"
	"movie-platform/services/processing/internal/processing"
	"movie-platform/shared/authmw"
	"movie-platform/shared/config"
	"movie-platform/shared/httpx"
)

func main() {
	config.LoadEnv()

	dbURL := os.Getenv("PROCESSING_DATABASE_URL")
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
	repo := processing.NewRepository(pool)

	worker := processing.NewWorker(repo, internalSecret)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
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
