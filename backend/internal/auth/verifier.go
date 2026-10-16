package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jws"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

type User struct {
	ID    string
	Name  string
	Image string
}

type Verifier struct {
	cache         *jwk.Cache
	url, iss, aud string
}

func NewVerifier(ctx context.Context, jwksURL, issuer, audience string) (*Verifier, error) {
	c := jwk.NewCache(ctx)
	if err := c.Register(jwksURL, jwk.WithMinRefreshInterval(15*time.Minute)); err != nil {
		return nil, err
	}
	// Don't hard-fail at startup if the web app isn't up yet; it's fetched lazily.
	return &Verifier{cache: c, url: jwksURL, iss: issuer, aud: audience}, nil
}

func (v *Verifier) Verify(ctx context.Context, token string) (User, error) {
	set, err := v.cache.Get(ctx, v.url)
	if err != nil {
		return User{}, fmt.Errorf("jwks: %w", err)
	}
	tok, err := jwt.Parse([]byte(token),
		jwt.WithKeySet(set, jws.WithInferAlgorithmFromKey(true)),
		jwt.WithIssuer(v.iss),
		jwt.WithAudience(v.aud),
		jwt.WithValidate(true),
	)
	if err != nil {
		return User{}, err
	}
	claims := tok.PrivateClaims()
	str := func(k string) string { s, _ := claims[k].(string); return s }
	return User{ID: tok.Subject(), Name: str("name"), Image: str("image")}, nil
}
