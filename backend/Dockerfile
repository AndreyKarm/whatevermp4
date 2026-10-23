FROM golang:1.27.2-alpine AS builder

WORKDIR /app

# Download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code and build (migrations are embedded in both binaries)
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o movie-server ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -o movie-worker ./cmd/worker

# API: no ffmpeg needed
FROM alpine:latest AS api
WORKDIR /app
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /app/movie-server .
EXPOSE 8088
CMD ["./movie-server"]

# Worker: Debian's ffmpeg includes NVENC (h264_nvenc), Alpine's doesn't.
# With HW_ENCODER=nvenc and docker-compose.nvidia.yml the GPU is used;
# otherwise it encodes with libx264.
FROM debian:trixie-slim AS worker
WORKDIR /app
RUN apt-get update \
	&& apt-get install -y --no-install-recommends ffmpeg ca-certificates tzdata \
	&& rm -rf /var/lib/apt/lists/*
COPY --from=builder /app/movie-worker .
CMD ["./movie-worker"]
