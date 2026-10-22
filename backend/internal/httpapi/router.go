package httpapi

import (
	"encoding/json"
	"liotom/whateverserver/internal/config"
	"liotom/whateverserver/internal/events"
	"liotom/whateverserver/internal/rooms"
	"liotom/whateverserver/internal/storage"
	"net/http"
	"net/url"
	"slices"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Server struct {
	db  *pgxpool.Pool
	st  *storage.Storage
	hub *rooms.Hub
	bus *events.Bus
	cfg config.Config
}

func NewRouter(
	db *pgxpool.Pool,
	st *storage.Storage,
	hub *rooms.Hub,
	bus *events.Bus,
	cfg config.Config,
) http.Handler {
	s := &Server{db: db, st: st, hub: hub, bus: bus, cfg: cfg}

	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(s.cors)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Signed HLS playlists, fMP4 segments, subtitles, fonts, and posters
	r.Get("/play/{token}/{id}/*", s.handlePlayback)

	// WebSocket watch-party endpoint (nickname via ?nickname=)
	r.Get("/api/rooms/{id}/ws", rooms.Handler(hub, cfg.AllowedOrigins))
	// WebSocket change notifications for the library and rooms pages
	r.Get("/api/events", s.handleEvents)

	r.Route("/api", func(r chi.Router) {
		// Media library
		r.Get("/media", s.listMedia)
		r.Get("/media/{id}", s.getMedia)
		r.Patch("/media/{id}", s.updateMedia)
		r.Delete("/media/{id}", s.deleteMedia)
		r.Post("/media/{id}/reprocess", s.reprocessMedia)
		r.Get("/media/{id}/download", s.downloadMedia)

		// Library folders (series, seasons, …)
		r.Get("/folders", s.listFolders)
		r.Post("/folders", s.createFolder)
		r.Patch("/folders/{id}", s.updateFolder)
		r.Delete("/folders/{id}", s.deleteFolder)

		// Direct S3 multipart uploads
		r.Post("/uploads/init", s.initUpload)
		r.Post("/uploads/{id}/presign", s.presignPart)
		r.Post("/uploads/{id}/complete", s.completeUpload)

		// Remote HLS (.m3u8) imports
		r.Post("/imports", s.createImport)

		// Torrents
		r.Get("/torrents", s.listTorrents)
		r.Post("/torrents", s.createTorrent)
		r.Delete("/torrents/{id}", s.cancelTorrent)

		// Watch-party rooms
		r.Get("/rooms", s.listRooms)
		r.Post("/rooms", s.createRoom)
		r.Get("/rooms/{id}", s.getRoom)
		r.Patch("/rooms/{id}", s.updateRoom)
		r.Delete("/rooms/{id}", s.deleteRoom)
	})

	return r
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && slices.Contains(s.cfg.AllowedOrigins, origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Range, X-Nickname")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Range, Accept-Ranges")
			// let browsers cache the preflight instead of sending one per request
			w.Header().Set("Access-Control-Max-Age", "600")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// nickname returns the self-chosen display name sent by the frontend
// (URI-encoded, since headers can't carry arbitrary UTF-8).
// No authentication is performed.
func nickname(r *http.Request) string {
	raw := r.Header.Get("X-Nickname")
	if s, err := url.QueryUnescape(raw); err == nil {
		raw = s
	}
	return rooms.CleanNickname(raw)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
