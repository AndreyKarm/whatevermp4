package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"movie-platform/services/rooms/internal/db"
	"movie-platform/services/rooms/internal/rooms"
	"movie-platform/shared/authmw"
	"movie-platform/shared/config"
)

func main() {
	config.LoadEnv()

	dbURL := os.Getenv("ROOMS_DATABASE_URL")
	if err := db.RunMigrations(dbURL); err != nil {
		log.Fatalf("migrations failed: %v", err)
	}

	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	jwks := authmw.NewJWKSClient(os.Getenv("AUTH_SERVICE_URL") + "/auth/.well-known/jwks.json")
	repo := rooms.NewRepository(pool)
	h := rooms.NewHandler(repo)
	hub := rooms.NewHub(repo)

	r := chi.NewRouter()

	// WebSocket route authenticates manually via ?token=... (see ws.go),
	// so it is not wrapped in authmw.RequireAuth.
	r.Get("/rooms/{code}/ws", hub.ServeWS(jwks))

	r.Group(func(r chi.Router) {
		r.Use(authmw.RequireAuth(jwks))
		r.Post("/rooms", h.CreateRoom)
		r.Post("/rooms/{code}/join", h.JoinRoom)
		r.Patch("/rooms/{id}/playback", h.UpdatePlayback)
	})

	port := os.Getenv("ROOMS_PORT")
	log.Printf("rooms service listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}
