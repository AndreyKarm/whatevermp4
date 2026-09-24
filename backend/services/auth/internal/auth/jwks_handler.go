package auth

import (
	"net/http"

	"movie-platform/shared/authmw"
	"movie-platform/shared/httpx"
)

// JWKS publishes the Auth service's public signing key(s) so other
// services can verify RS256 access tokens. On key rotation, keep the old
// public key listed here until every token signed with it has expired,
// then remove it.
func (h *Handler) JWKS(w http.ResponseWriter, r *http.Request) {
	jwk := authmw.PublicKeyToJWK(h.keys.Kid, &h.keys.Private.PublicKey)
	httpx.WriteJSON(w, http.StatusOK, authmw.JWKSet{Keys: []authmw.JWK{jwk}})
}
