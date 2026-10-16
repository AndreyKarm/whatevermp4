package auth

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type User struct {
	ID    string
	Name  string
	Image string
}

type Verifier struct {
	db *pgxpool.Pool
}

func NewVerifier(db *pgxpool.Pool) *Verifier {
	return &Verifier{db: db}
}

func (v *Verifier) Verify(ctx context.Context, rawToken string) (User, error) {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return User{}, errors.New("empty token")
	}
	if unescaped, err := url.QueryUnescape(rawToken); err == nil {
		rawToken = unescaped
	}
	// BetterAuth signed cookies have the format "<session_token>.<hmac_sig>".
	// The database stores the raw <session_token> part before the dot.
	token, _, _ := strings.Cut(rawToken, ".")
	if token == "" {
		return User{}, errors.New("invalid token")
	}

	var u User
	err := v.db.QueryRow(ctx, `
		SELECT u.id, u.name, COALESCE(u.image, '')
		FROM "session" s
		JOIN "user" u ON u.id = s.user_id
		WHERE s.token = $1 AND s.expires_at > now()`,
		token,
	).Scan(&u.ID, &u.Name, &u.Image)
	if err != nil {
		return User{}, err
	}
	return u, nil
}

// ExtractToken checks "Authorization: Bearer <token>", "?token=", and BetterAuth cookies.
func ExtractToken(headerAuth, queryToken string, cookies func(string) string) string {
	if tok, ok := strings.CutPrefix(headerAuth, "Bearer "); ok && tok != "" {
		return tok
	}
	if queryToken != "" {
		return queryToken
	}
	if c := cookies("__Secure-better-auth.session_token"); c != "" {
		return c
	}
	return cookies("better-auth.session_token")
}
