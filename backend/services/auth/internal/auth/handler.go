package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"movie-platform/shared/authmw"
	"movie-platform/shared/httpx"
)

type Handler struct {
	repo *Repository
	keys *KeyPair
}

func NewHandler(repo *Repository, keys *KeyPair) *Handler {
	return &Handler{repo: repo, keys: keys}
}

type authResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	User         struct {
		ID       string `json:"id"`
		Email    string `json:"email"`
		Role     string `json:"role"`
		Username string `json:"username"`
	} `json:"user"`
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid body")
		return
	}

	email := strings.ToLower(strings.TrimSpace(body.Email))
	username := strings.TrimSpace(body.Username)

	if email == "" || !strings.Contains(email, "@") {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid email", "field": "email"})
		return
	}
	if len(body.Password) < 8 || len(body.Password) > 72 {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "password must be 8-72 characters", "field": "password"})
		return
	}
	if len(username) < 2 || len(username) > 32 {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "username must be 2-32 characters", "field": "username"})
		return
	}

	hash, err := HashPassword(body.Password)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	user, err := h.repo.CreateUser(r.Context(), email, username, hash)
	if err != nil {
		switch {
		case errors.Is(err, ErrEmailTaken):
			httpx.WriteJSON(w, http.StatusConflict, map[string]string{"error": "email already in use", "field": "email"})
		case errors.Is(err, ErrUsernameTaken):
			httpx.WriteJSON(w, http.StatusConflict, map[string]string{"error": "username already taken", "field": "username"})
		default:
			httpx.Error(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	h.issueTokens(w, r, user)
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid body")
		return
	}

	email := strings.ToLower(strings.TrimSpace(body.Email))

	user, err := h.repo.GetUserByEmail(r.Context(), email)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.Error(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	ok, err := VerifyPassword(body.Password, user.PasswordHash)
	if err != nil || !ok {
		httpx.Error(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	h.issueTokens(w, r, user)
}

func (h *Handler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	claims, ok := authmw.ClaimsFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	var body struct {
		Username *string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid body")
		return
	}

	if body.Username != nil && (len(*body.Username) < 2 || len(*body.Username) > 32) {
		httpx.Error(w, http.StatusBadRequest, "nickname must be 2-32 characters")
		return
	}

	if err := h.repo.UpdateProfile(r.Context(), claims.UserID, body.Username, nil); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "update failed")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refreshToken"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.RefreshToken == "" {
		httpx.Error(w, http.StatusBadRequest, "missing refresh token")
		return
	}

	hash := HashRefreshToken(body.RefreshToken)
	userID, sessionID, err := h.repo.GetSessionByHash(r.Context(), hash)
	if err != nil {
		httpx.Error(w, http.StatusUnauthorized, "invalid or expired session")
		return
	}

	_ = h.repo.RevokeSession(r.Context(), sessionID)

	user, err := h.repo.GetUserByID(r.Context(), userID)
	if err != nil {
		httpx.Error(w, http.StatusUnauthorized, "user not found")
		return
	}

	h.issueTokens(w, r, user)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refreshToken"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.RefreshToken != "" {
		_ = h.repo.RevokeSessionByHash(r.Context(), HashRefreshToken(body.RefreshToken))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	claims, ok := authmw.ClaimsFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{
		"id": claims.UserID, "email": claims.Email, "role": claims.Role,
	})
}

// UpdateUserRole is called by an admin. Since role changes affect access
// control everywhere, this stays in the Auth service, gated by RequireRole.
func (h *Handler) UpdateUserRole(w http.ResponseWriter, r *http.Request) {
	id := chiURLParam(r, "id")
	var body struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Role != "admin" && body.Role != "user" {
		httpx.Error(w, http.StatusBadRequest, "invalid role")
		return
	}
	if err := h.repo.UpdateUserRole(r.Context(), id, body.Role); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "update failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) issueTokens(w http.ResponseWriter, r *http.Request, user *User) {
	username := ""
	if user.Username != nil {
		username = *user.Username
	}

	access, err := GenerateAccessToken(h.keys, user.ID, user.Email, user.Role, username)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "token error")
		return
	}

	refreshPlain, refreshHash, err := GenerateRefreshToken()
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "token error")
		return
	}

	if err := h.repo.CreateSession(r.Context(), user.ID, refreshHash, r.UserAgent(), r.RemoteAddr); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "session error")
		return
	}

	resp := authResponse{AccessToken: access, RefreshToken: refreshPlain}
	resp.User.ID = user.ID
	resp.User.Email = user.Email
	resp.User.Role = user.Role
	resp.User.Username = username
	httpx.WriteJSON(w, http.StatusOK, resp)
}
