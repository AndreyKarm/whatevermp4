package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrEmailTaken    = errors.New("email already in use")
	ErrUsernameTaken = errors.New("username already taken")
	ErrNotFound      = errors.New("not found")
)

type User struct {
	ID           string
	Email        string
	PasswordHash string
	Role         string
	Username     *string
	AvatarKey    *string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CreateUser(ctx context.Context, email, username, hash string) (*User, error) {
	u := &User{Email: email, Username: &username, PasswordHash: hash, Role: "user"}

	err := r.db.QueryRow(ctx,
		`insert into users (email, username, password_hash, role)
		 values ($1, $2, $3, $4)
		 returning id, created_at, updated_at`,
		email, username, hash, u.Role,
	).Scan(&u.ID, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			switch pgErr.ConstraintName {
			case "users_email_key":
				return nil, ErrEmailTaken
			case "users_username_key":
				return nil, ErrUsernameTaken
			}
		}
		return nil, err
	}
	return u, nil
}

func (r *Repository) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	var u User
	err := r.db.QueryRow(ctx,
		`select id, email, password_hash, role from users where email = $1`, email,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

func (r *Repository) GetUserByID(ctx context.Context, id string) (*User, error) {
	var u User
	err := r.db.QueryRow(ctx,
		`select id, email, password_hash, role, username, avatar_key, created_at, updated_at
		 from users where id = $1`, id,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &u.Username, &u.AvatarKey, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

func (r *Repository) UpdateProfile(ctx context.Context, id string, username, avatarKey *string) error {
	sets := []string{}
	args := []any{}
	i := 1

	if username != nil {
		sets = append(sets, fmt.Sprintf("username = $%d", i))
		args = append(args, *username)
		i++
	}
	if avatarKey != nil {
		sets = append(sets, fmt.Sprintf("avatar_key = $%d", i))
		args = append(args, *avatarKey)
		i++
	}
	sets = append(sets, "updated_at = now()")

	query := fmt.Sprintf("update users set %s where id = $%d", strings.Join(sets, ", "), i)
	args = append(args, id)

	_, err := r.db.Exec(ctx, query, args...)
	return err
}

func (r *Repository) UpdateUserRole(ctx context.Context, id, role string) error {
	_, err := r.db.Exec(ctx,
		`update users set role = $1, updated_at = now() where id = $2`, role, id)
	return err
}

func (r *Repository) CreateSession(ctx context.Context, userID, refreshHash, userAgent, ip string) error {
	_, err := r.db.Exec(ctx,
		`insert into sessions (user_id, refresh_token_hash, user_agent, ip_address, expires_at)
		 values ($1, $2, $3, $4, $5)`,
		userID, refreshHash, userAgent, ip, time.Now().Add(RefreshTokenTTL),
	)
	return err
}

func (r *Repository) GetSessionByHash(ctx context.Context, hash string) (userID string, sessionID string, err error) {
	err = r.db.QueryRow(ctx,
		`select user_id, id from sessions
		 where refresh_token_hash = $1 and revoked_at is null and expires_at > now()`,
		hash,
	).Scan(&userID, &sessionID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", ErrNotFound
		}
		return "", "", err
	}
	return userID, sessionID, nil
}

func (r *Repository) RevokeSession(ctx context.Context, sessionID string) error {
	_, err := r.db.Exec(ctx, `update sessions set revoked_at = now() where id = $1`, sessionID)
	return err
}

func (r *Repository) RevokeSessionByHash(ctx context.Context, hash string) error {
	_, err := r.db.Exec(ctx, `update sessions set revoked_at = now() where refresh_token_hash = $1`, hash)
	return err
}
