module movie-platform/services/rooms

go 1.27.1

replace movie-platform/shared => ../../shared

require (
	github.com/go-chi/chi/v5 v5.3.2
	github.com/golang-migrate/migrate/v4 v4.20.1
	github.com/gorilla/websocket v1.5.3
	github.com/jackc/pgx/v5 v5.11.0
	movie-platform/shared v0.0.0
)

require (
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/joho/godotenv v1.5.1 // indirect
	github.com/lib/pq v1.10.9 // indirect
	golang.org/x/sync v0.21.0 // indirect
	golang.org/x/text v0.38.0 // indirect
)
