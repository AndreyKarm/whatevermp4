package auth

import "github.com/alexedwards/argon2id"

func HashPassword(plain string) (string, error) {
	return argon2id.CreateHash(plain, argon2id.DefaultParams)
}

func VerifyPassword(plain, hash string) (bool, error) {
	match, _, err := argon2id.CheckHash(plain, hash)
	return match, err
}
