module movie-platform/services/gateway

replace movie-platform/shared => ../../shared

go 1.27.1

require (
	github.com/go-chi/chi/v5 v5.3.2
	movie-platform/shared v0.0.0
)

require (
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/joho/godotenv v1.5.1 // indirect
)
