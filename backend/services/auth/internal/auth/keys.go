package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"log"
	"os"
)

// KeyPair holds the Auth service's RSA signing key. This is the only place
// in the system that ever touches the private key; every other service
// verifies tokens against the public half via the JWKS endpoint.
type KeyPair struct {
	Private *rsa.PrivateKey
	Kid     string
}

// LoadOrGenerateKeyPair loads a PEM-encoded RSA private key from the
// AUTH_JWT_PRIVATE_KEY env var (base64-encoded PKCS1 PEM) if present,
// otherwise generates a fresh in-memory keypair. Always set the env var in
// production — an ephemeral key invalidates all outstanding tokens on
// restart and breaks JWKS consistency across multiple Auth replicas.
func LoadOrGenerateKeyPair() (*KeyPair, error) {
	if encoded := os.Getenv("AUTH_JWT_PRIVATE_KEY"); encoded != "" {
		pemBytes, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, err
		}
		block, _ := pem.Decode(pemBytes)
		if block == nil {
			return nil, errors.New("failed to decode PEM block for AUTH_JWT_PRIVATE_KEY")
		}
		key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		return &KeyPair{Private: key, Kid: kidFor(&key.PublicKey)}, nil
	}

	log.Println("AUTH_JWT_PRIVATE_KEY not set; generating an ephemeral RSA key (dev only)")
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	return &KeyPair{Private: key, Kid: kidFor(&key.PublicKey)}, nil
}

func kidFor(pub *rsa.PublicKey) string {
	sum := sha256.Sum256(pub.N.Bytes())
	return base64.RawURLEncoding.EncodeToString(sum[:8])
}
