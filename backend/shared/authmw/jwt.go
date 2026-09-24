package authmw

import (
	"context"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// Claims mirrors what the Auth service embeds in access tokens.
// Every service that needs to authenticate requests uses this same shape,
// so they must be configured with the same AUTH_JWT_SECRET as the Auth
// service (until it is upgraded to RS256/JWKS for asymmetric verification).
type Claims struct {
	UserID   string `json:"sub"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

type ctxKey string

const claimsKey ctxKey = "claims"

func ClaimsFromContext(ctx context.Context) (*Claims, bool) {
	c, ok := ctx.Value(claimsKey).(*Claims)
	return c, ok
}

func ParseAccessToken(jwks *JWKSClient, tokenStr string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		kid, ok := t.Header["kid"].(string)
		if !ok || kid == "" {
			return nil, jwt.ErrTokenMalformed
		}
		return jwks.PublicKey(kid)
	}, jwt.WithValidMethods([]string{"RS256"}))
	if err != nil {
		return nil, err
	}
	return claims, nil
}

func RequireAuth(jwks *JWKSClient) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			token := strings.TrimPrefix(header, "Bearer ")
			if token == "" || token == header {
				http.Error(w, "missing bearer token", http.StatusUnauthorized)
				return
			}
			claims, err := ParseAccessToken(jwks, token)
			if err != nil {
				http.Error(w, "invalid or expired token", http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), claimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RequireRole(roles ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(roles))
	for _, r := range roles {
		allowed[r] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := ClaimsFromContext(r.Context())
			if !ok || !allowed[claims.Role] {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
