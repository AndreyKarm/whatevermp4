package config

import (
	"log"

	"github.com/joho/godotenv"
)

// LoadEnv loads a .env file from the current working directory if present.
// In containers/CI, real environment variables are used instead.
func LoadEnv() {
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file found, relying on real environment variables")
	}
}
