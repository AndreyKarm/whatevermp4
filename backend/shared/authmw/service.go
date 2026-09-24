package authmw

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ServiceClaims identifies which internal service issued a token, used for
// service-to-service calls (e.g. Processing -> Catalog) instead of an
// end-user JWT. Tokens are short-lived and signed with a separate secret
// (INTERNAL_SERVICE_SECRET) that only backend services hold — it is never
// issued to or accepted from browsers/mobile clients.
type ServiceClaims struct {
	Service string `json:"svc"`
	jwt.RegisteredClaims
}

const ServiceTokenTTL = 60 * time.Second

func GenerateServiceToken(secret []byte, serviceName string) (string, error) {
	claims := ServiceClaims{
		Service: serviceName,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ServiceTokenTTL)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secret)
}

type serviceCtxKey string

const serviceClaimsKey serviceCtxKey = "serviceClaims"

func ServiceClaimsFromContext(ctx context.Context) (*ServiceClaims, bool) {
	c, ok := ctx.Value(serviceClaimsKey).(*ServiceClaims)
	return c, ok
}

// RequireServiceAuth verifies an internal service token instead of an
// end-user access token. Use only on routes that must never be reachable
// by end users, even admins — e.g. Processing reporting job status back
// to Catalog.
func RequireServiceAuth(secret []byte, allowed ...string) func(http.Handler) http.Handler {
	allowedSet := make(map[string]bool, len(allowed))
	for _, s := range allowed {
		allowedSet[s] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			tokenStr := strings.TrimPrefix(header, "Bearer ")
			if tokenStr == "" || tokenStr == header {
				http.Error(w, "missing service token", http.StatusUnauthorized)
				return
			}

			claims := &ServiceClaims{}
			_, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
				return secret, nil
			})
			if err != nil {
				http.Error(w, "invalid or expired service token", http.StatusUnauthorized)
				return
			}

			if len(allowedSet) > 0 && !allowedSet[claims.Service] {
				http.Error(w, "service not permitted", http.StatusForbidden)
				return
			}

			ctx := context.WithValue(r.Context(), serviceClaimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
