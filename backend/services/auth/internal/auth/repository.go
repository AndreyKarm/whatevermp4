package auth

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/driver/pgdriver"
)

var (
	ErrEmailTaken    = errors.New("email already in use")
	ErrUsernameTaken = errors.New("username already taken")
	ErrNotFound      = errors.New("not found")
)

type User struct {
	bun.BaseModel `bun:"table:users,alias:u"`

	ID           string    `bun:"id,pk,type:uuid,nullzero,default:gen_random_uuid()"`
	Email        string    `bun:"email,notnull"`
	PasswordHash string    `bun:"password_hash,notnull"`
	Role         string    `bun:"role,notnull,default:'user'"`
	Username     *string   `bun:"username"`
	AvatarKey    *string   `bun:"avatar_key"`
	CreatedAt    time.Time `bun:"created_at,nullzero,notnull,default:now()"`
	UpdatedAt    time.Time `bun:"updated_at,nullzero,notnull,default:now()"`
}

type Session struct {
	bun.BaseModel `bun:"table:sessions,alias:s"`

	ID               string     `bun:"id,pk,type:uuid,nullzero,default:gen_random_uuid()"`
	UserID           string     `bun:"user_id,type:uuid,notnull"`
	RefreshTokenHash string     `bun:"refresh_token_hash,notnull"`
	UserAgent        string     `bun:"user_agent"`
	IPAddress        string     `bun:"ip_address"`
	ExpiresAt        time.Time  `bun:"expires_at,notnull"`
	RevokedAt        *time.Time `bun:"revoked_at"`
	CreatedAt        time.Time  `bun:"created_at,nullzero,notnull,default:now()"`
}

type Repository struct {
	db *bun.DB
}

func NewRepository(db *bun.DB) *Repository {
	return &Repository{db: db}
}

// mapUniqueViolation translates Postgres unique violations (23505) into
// domain errors. Returns the original error if it isn't one we know.
func mapUniqueViolation(err error) error {
	var pgErr pgdriver.Error
	if errors.As(err, &pgErr) && pgErr.Field('C') == "23505" {
		switch pgErr.Field('n') { // constraint / index name
		case "users_email_key":
			return ErrEmailTaken
		case "users_username_lower_idx":
			return ErrUsernameTaken
		}
	}
	return err
}

func (r *Repository) CreateUser(ctx context.Context, email, username, hash string) (*User, error) {
	u := &User{
		Email:        email,
		Username:     &username,
		PasswordHash: hash,
		Role:         "user",
	}

	_, err := r.db.NewInsert().
		Model(u).
		Returning("id, created_at, updated_at").
		Exec(ctx)
	if err != nil {
		return nil, mapUniqueViolation(err)
	}
	return u, nil
}

func (r *Repository) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	u := new(User)
	err := r.db.NewSelect().
		Model(u).
		Where("email = ?", email).
		Limit(1).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return u, nil
}

func (r *Repository) GetUserByID(ctx context.Context, id string) (*User, error) {
	u := new(User)
	err := r.db.NewSelect().
		Model(u).
		Where("id = ?", id).
		Limit(1).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return u, nil
}

func (r *Repository) UpdateProfile(ctx context.Context, id string, username, avatarKey *string) error {
	q := r.db.NewUpdate().
		Model((*User)(nil)).
		Set("updated_at = now()").
		Where("id = ?", id)

	if username != nil {
		q = q.Set("username = ?", *username)
	}
	if avatarKey != nil {
		q = q.Set("avatar_key = ?", *avatarKey)
	}

	_, err := q.Exec(ctx)
	return mapUniqueViolation(err)
}

func (r *Repository) UpdateUserRole(ctx context.Context, id, role string) error {
	_, err := r.db.NewUpdate().
		Model((*User)(nil)).
		Set("role = ?", role).
		Set("updated_at = now()").
		Where("id = ?", id).
		Exec(ctx)
	return err
}

func (r *Repository) CreateSession(ctx context.Context, userID, refreshHash, userAgent, ip string) error {
	s := &Session{
		UserID:           userID,
		RefreshTokenHash: refreshHash,
		UserAgent:        userAgent,
		IPAddress:        ip,
		ExpiresAt:        time.Now().Add(RefreshTokenTTL),
	}
	_, err := r.db.NewInsert().Model(s).Exec(ctx)
	return err
}

func (r *Repository) GetSessionByHash(ctx context.Context, hash string) (userID string, sessionID string, err error) {
	s := new(Session)
	err = r.db.NewSelect().
		Model(s).
		Column("id", "user_id").
		Where("refresh_token_hash = ?", hash).
		Where("revoked_at IS NULL").
		Where("expires_at > now()").
		Limit(1).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", "", ErrNotFound
		}
		return "", "", err
	}
	return s.UserID, s.ID, nil
}

func (r *Repository) RevokeSession(ctx context.Context, sessionID string) error {
	_, err := r.db.NewUpdate().
		Model((*Session)(nil)).
		Set("revoked_at = now()").
		Where("id = ?", sessionID).
		Exec(ctx)
	return err
}

func (r *Repository) RevokeSessionByHash(ctx context.Context, hash string) error {
	_, err := r.db.NewUpdate().
		Model((*Session)(nil)).
		Set("revoked_at = now()").
		Where("refresh_token_hash = ?", hash).
		Exec(ctx)
	return err
}
