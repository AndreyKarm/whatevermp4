package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"

	"movie-platform/services/auth/internal/auth"
	"movie-platform/services/auth/internal/db"
	"movie-platform/shared/authmw"
	"movie-platform/shared/config"
)

func main() {
	config.LoadEnv()

	ctx := context.Background()

	dbURL := os.Getenv("AUTH_DATABASE_URL")

	bunDB, err := auth.InitDB(dbURL)
	if err != nil {
		log.Fatal(err)
	}
	defer bunDB.Close()

	if err := db.RunMigrations(ctx, bunDB); err != nil {
		log.Fatalf("migrations failed: %v", err)
	}

	keys, err := auth.LoadOrGenerateKeyPair()
	if err != nil {
		log.Fatalf("failed to load signing key: %v", err)
	}

	jwks := authmw.NewJWKSClient(os.Getenv("AUTH_SERVICE_URL") + "/auth/.well-known/jwks.json")

	repo := auth.NewRepository(bunDB)
	h := auth.NewHandler(repo, keys)

	r := chi.NewRouter()

	r.Get("/auth/.well-known/jwks.json", h.JWKS)

	r.Post("/auth/register", h.Register)
	r.Post("/auth/login", h.Login)
	r.Post("/auth/refresh", h.Refresh)
	r.Post("/auth/logout", h.Logout)

	r.Group(func(r chi.Router) {
		r.Use(authmw.RequireAuth(jwks))
		r.Get("/auth/me", h.Me)
		r.Patch("/auth/me", h.UpdateProfile)

		r.Group(func(r chi.Router) {
			r.Use(authmw.RequireRole("admin"))
			r.Patch("/auth/admin/users/{id}/role", h.UpdateUserRole)
		})
	})

	port := os.Getenv("AUTH_PORT")
	log.Printf("auth service listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}
