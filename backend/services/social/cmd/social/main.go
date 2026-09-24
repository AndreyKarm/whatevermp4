package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"movie-platform/services/social/internal/db"
	"movie-platform/services/social/internal/social"
	"movie-platform/shared/authmw"
	"movie-platform/shared/config"
)

func main() {
	config.LoadEnv()

	dbURL := os.Getenv("SOCIAL_DATABASE_URL")
	if err := db.RunMigrations(dbURL); err != nil {
		log.Fatalf("migrations failed: %v", err)
	}

	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	jwks := authmw.NewJWKSClient(os.Getenv("AUTH_SERVICE_URL") + "/auth/.well-known/jwks.json")

	commentsRepo := social.NewCommentsRepository(pool)
	commentsHandler := social.NewCommentsHandler(commentsRepo)

	ratingsRepo := social.NewRatingsRepository(pool)
	ratingsHandler := social.NewRatingsHandler(ratingsRepo)

	watchlistRepo := social.NewWatchlistRepository(pool)
	watchlistHandler := social.NewWatchlistHandler(watchlistRepo)

	r := chi.NewRouter()

	r.Get("/social/comments/title/{titleId}", commentsHandler.ListByTitle)

	r.Group(func(r chi.Router) {
		r.Use(authmw.RequireAuth(jwks))
		r.Post("/social/comments", commentsHandler.Create)
		r.Put("/social/ratings", ratingsHandler.Upsert)
		r.Put("/social/watchlist", watchlistHandler.Upsert)
	})

	port := os.Getenv("SOCIAL_PORT")
	log.Printf("social service listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}
