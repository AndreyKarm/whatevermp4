package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"github.com/go-chi/chi/v5"
	"github.com/joho/godotenv"

	"liotom/movie-server/internal/auth"
	"liotom/movie-server/internal/db"
)

func loadRootEnv() {
	_, thisFile, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
	envPath := filepath.Join(root, ".env")

	if err := godotenv.Load(envPath); err != nil {
		log.Printf("no .env file found at %s, relying on real environment variables", envPath)
	}
}

func main() {
	loadRootEnv()

	dbURL := os.Getenv("DATABASE_URL")

	if err := db.RunMigrations(dbURL); err != nil {
		log.Fatalf("migrations failed: %v", err)
	}

	pool, err := auth.InitDB(dbURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	secret := []byte(os.Getenv("AUTH_JWT_SECRET"))
	repo := auth.NewRepository(pool)
	authHandler := auth.NewHandler(repo, secret)

	r := chi.NewRouter()

	// Auth
	r.Patch("/auth/me", authHandler.UpdateProfile)
	r.Post("/auth/register", authHandler.Register)
	r.Post("/auth/login", authHandler.Login)
	r.Post("/auth/refresh", authHandler.Refresh)
	r.Post("/auth/logout", authHandler.Logout)

	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAuth(secret))
		r.Get("/auth/me", authHandler.Me)

		r.Group(func(r chi.Router) {
			r.Use(auth.RequireRole("admin"))
			r.Patch("/admin/users/{id}/role", func(w http.ResponseWriter, r *http.Request) {
				// update role via repo.UpdateUserRole
			})
		})
	})

	log.Printf("listening on :%s", os.Getenv("BACKEND_PORT"))
	http.ListenAndServe(":"+os.Getenv("BACKEND_PORT"), r)
}
