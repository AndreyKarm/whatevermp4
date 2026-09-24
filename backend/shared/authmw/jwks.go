package authmw

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"
)

// JWK is a minimal JSON Web Key representation for RSA public keys, enough
// to verify RS256-signed access tokens.
type JWK struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type JWKSet struct {
	Keys []JWK `json:"keys"`
}

func PublicKeyToJWK(kid string, pub *rsa.PublicKey) JWK {
	return JWK{
		Kty: "RSA",
		Kid: kid,
		Use: "sig",
		Alg: "RS256",
		N:   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}
}

func jwkToPublicKey(jwk JWK) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(jwk.N)
	if err != nil {
		return nil, err
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(jwk.E)
	if err != nil {
		return nil, err
	}
	n := new(big.Int).SetBytes(nBytes)
	e := new(big.Int).SetBytes(eBytes)
	return &rsa.PublicKey{N: n, E: int(e.Int64())}, nil
}

// JWKSClient fetches and caches the Auth service's public signing keys, so
// other services can verify RS256 access tokens without ever holding the
// private signing key. Keys are cached with a TTL and refreshed on-demand
// if an unknown `kid` is seen (e.g. right after key rotation).
type JWKSClient struct {
	jwksURL string
	http    *http.Client

	mu        sync.RWMutex
	keys      map[string]*rsa.PublicKey
	fetchedAt time.Time
	ttl       time.Duration
}

func NewJWKSClient(jwksURL string) *JWKSClient {
	return &JWKSClient{
		jwksURL: jwksURL,
		http:    &http.Client{Timeout: 5 * time.Second},
		keys:    make(map[string]*rsa.PublicKey),
		ttl:     10 * time.Minute,
	}
}

func (c *JWKSClient) refresh() error {
	resp, err := c.http.Get(c.jwksURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jwks fetch failed: status %d", resp.StatusCode)
	}

	var set JWKSet
	if err := json.NewDecoder(resp.Body).Decode(&set); err != nil {
		return err
	}

	keys := make(map[string]*rsa.PublicKey, len(set.Keys))
	for _, jwk := range set.Keys {
		pub, err := jwkToPublicKey(jwk)
		if err != nil {
			continue
		}
		keys[jwk.Kid] = pub
	}

	c.mu.Lock()
	c.keys = keys
	c.fetchedAt = time.Now()
	c.mu.Unlock()
	return nil
}

// PublicKey returns the RSA public key for the given kid, refreshing the
// cache if it is stale or the kid is unknown (handles key rotation).
func (c *JWKSClient) PublicKey(kid string) (*rsa.PublicKey, error) {
	c.mu.RLock()
	key, ok := c.keys[kid]
	stale := time.Since(c.fetchedAt) > c.ttl
	c.mu.RUnlock()

	if ok && !stale {
		return key, nil
	}

	if err := c.refresh(); err != nil {
		if ok {
			return key, nil // serve stale-but-known key rather than fail
		}
		return nil, err
	}

	c.mu.RLock()
	key, ok = c.keys[kid]
	c.mu.RUnlock()
	if !ok {
		return nil, errors.New("unknown key id")
	}
	return key, nil
}
