package config

import (
	"os"
	"slices"
	"strings"
)

type Config struct {
	Addr             string
	DatabaseURL      string
	S3Endpoint       string // internal, e.g. http://rustfs:9000
	S3PublicEndpoint string // browser-reachable, used for presigned URLs
	S3Region         string
	S3AccessKey      string
	S3SecretKey      string
	S3Bucket         string
	WorkDir          string
	AllowedOrigins   []string
	PlaybackSecret   string
	AdminIDs         []string
}

func (c Config) IsAdmin(userID string) bool {
	return userID != "" && slices.Contains(c.AdminIDs, userID)
}

func get(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func Load() Config {
	return Config{
		Addr:             get("ADDR", ":8080"),
		DatabaseURL:      get("DATABASE_URL", ""),
		S3Endpoint:       get("S3_ENDPOINT", "http://localhost:9000"),
		S3PublicEndpoint: get("S3_PUBLIC_ENDPOINT", "http://localhost:9000"),
		S3Region:         get("S3_REGION", "us-east-1"),
		S3AccessKey:      get("S3_ACCESS_KEY", ""),
		S3SecretKey:      get("S3_SECRET_KEY", ""),
		S3Bucket:         get("S3_BUCKET", "media"),
		WorkDir:          get("WORK_DIR", "/tmp/work"),
		AllowedOrigins:   splitCSV(get("ALLOWED_ORIGINS", "http://localhost:5173")),
		PlaybackSecret:   get("PLAYBACK_SECRET", "change-me"),
		AdminIDs:         splitCSV(get("ADMIN_IDS", "")),
	}
}
