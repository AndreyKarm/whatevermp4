package main

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"

	"github.com/go-chi/chi/v5"

	"movie-platform/shared/authmw"
	"movie-platform/shared/config"
)

func mustProxy(target string) *httputil.ReverseProxy {
	u, err := url.Parse(target)
	if err != nil {
		log.Fatalf("invalid service url %s: %v", target, err)
	}
	return httputil.NewSingleHostReverseProxy(u)
}

func mount(r chi.Router, prefix string, proxy *httputil.ReverseProxy) {
	strip := http.StripPrefix(prefix, proxy)
	r.Handle(prefix+"/*", strip)
}

func main() {
	config.LoadEnv()
	jwks := authmw.NewJWKSClient(os.Getenv("AUTH_SERVICE_URL") + "/auth/.well-known/jwks.json")

	authProxy := mustProxy(os.Getenv("AUTH_SERVICE_URL"))
	catalogProxy := mustProxy(os.Getenv("CATALOG_SERVICE_URL"))
	processingProxy := mustProxy(os.Getenv("PROCESSING_SERVICE_URL"))
	roomsProxy := mustProxy(os.Getenv("ROOMS_SERVICE_URL"))
	socialProxy := mustProxy(os.Getenv("SOCIAL_SERVICE_URL"))

	r := chi.NewRouter()

	// Public auth routes (register/login/refresh/logout) pass straight through.
	mount(r, "/auth", authProxy)

	// Everything else requires a valid access token; per-route role checks
	// happen inside each downstream service using the same JWT claims.
	r.Group(func(r chi.Router) {
		r.Use(authmw.RequireAuth(jwks))
		mount(r, "/catalog", catalogProxy)
		mount(r, "/processing", processingProxy)
		mount(r, "/rooms", roomsProxy)
		mount(r, "/social", socialProxy)
	})

	port := os.Getenv("BACKEND_PORT")
	log.Printf("gateway listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}
