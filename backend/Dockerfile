FROM golang:1.27.2-alpine AS builder

WORKDIR /app

# Download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o movie-server ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -o movie-worker ./cmd/worker

# Production stage
FROM alpine:latest
WORKDIR /app

# Install runtime dependencies (ffmpeg, ffprobe, ca-certificates, tzdata)
RUN apk add --no-cache ffmpeg ca-certificates tzdata

COPY --from=builder /app/movie-server .
COPY --from=builder /app/movie-worker .
COPY migrations/ ./migrations/

# Expose the default ADDR port
EXPOSE 8088

CMD ["./movie-server"]