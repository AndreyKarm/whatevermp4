package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

type Handler struct {
	repo   *Repository
	secret []byte
}

func NewHandler(repo *Repository, secret []byte) *Handler {
	return &Handler{repo: repo, secret: secret}
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}

	email := strings.ToLower(strings.TrimSpace(body.Email))
	username := strings.TrimSpace(body.Username)

	if email == "" || !strings.Contains(email, "@") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid email", "field": "email"})
		return
	}
	if len(body.Password) < 8 || len(body.Password) > 72 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password must be 8-72 characters", "field": "password"})
		return
	}
	if len(username) < 2 || len(username) > 32 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username must be 2-32 characters", "field": "username"})
		return
	}

	hash, err := HashPassword(body.Password)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	user, err := h.repo.CreateUser(r.Context(), email, username, hash)
	if err != nil {
		switch {
		case errors.Is(err, ErrEmailTaken):
			writeJSON(w, http.StatusConflict, map[string]string{"error": "email already in use", "field": "email"})
		case errors.Is(err, ErrUsernameTaken):
			writeJSON(w, http.StatusConflict, map[string]string{"error": "username already taken", "field": "username"})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
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
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}

	email := strings.ToLower(strings.TrimSpace(body.Email))

	user, err := h.repo.GetUserByEmail(r.Context(), email)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	ok, err := VerifyPassword(body.Password, user.PasswordHash)
	if err != nil || !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}

	h.issueTokens(w, r, user)
}

func (h *Handler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
		return
	}

	var body struct {
		Username *string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}

	if body.Username != nil && (len(*body.Username) < 2 || len(*body.Username) > 32) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "nickname must be 2-32 characters"})
		return
	}

	if err := h.repo.UpdateProfile(r.Context(), claims.UserID, body.Username, nil); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "update failed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refreshToken"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.RefreshToken == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing refresh token"})
		return
	}

	hash := HashRefreshToken(body.RefreshToken)
	userID, sessionID, err := h.repo.GetSessionByHash(r.Context(), hash)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired session"})
		return
	}

	// Rotation: revoke the used token immediately, issue a brand-new one.
	_ = h.repo.RevokeSession(r.Context(), sessionID)

	user, err := h.repo.GetUserByID(r.Context(), userID)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "user not found"})
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
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"id": claims.UserID, "email": claims.Email, "role": claims.Role,
	})
}

func (h *Handler) issueTokens(w http.ResponseWriter, r *http.Request, user *User) {
	username := ""
	if user.Username != nil {
		username = *user.Username
	}

	access, err := GenerateAccessToken(h.secret, user.ID, user.Email, user.Role, username)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "token error"})
		return
	}

	refreshPlain, refreshHash, err := GenerateRefreshToken()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "token error"})
		return
	}

	if err := h.repo.CreateSession(r.Context(), user.ID, refreshHash, r.UserAgent(), r.RemoteAddr); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "session error"})
		return
	}

	resp := authResponse{AccessToken: access, RefreshToken: refreshPlain}
	resp.User.ID = user.ID
	resp.User.Email = user.Email
	resp.User.Role = user.Role
	resp.User.Username = username
	writeJSON(w, http.StatusOK, resp)
}
