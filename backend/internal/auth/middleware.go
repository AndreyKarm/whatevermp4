package auth

import (
	"context"
	"net/http"
	"strings"
)

type ctxKey struct{}

func FromContext(ctx context.Context) User {
	u, _ := ctx.Value(ctxKey{}).(User)
	return u
}

func Middleware(v *Verifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Get("Authorization")
			tok, ok := strings.CutPrefix(h, "Bearer ")
			if !ok || tok == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			u, err := v.Verify(r.Context(), tok)
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, u)))
		})
	}
}
