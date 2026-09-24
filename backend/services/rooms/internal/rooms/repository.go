package rooms

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

type Room struct {
	ID              string  `json:"id"`
	Code            string  `json:"code"`
	HostUserID      string  `json:"hostUserId"`
	MediaVersionID  string  `json:"mediaVersionId"`
	PositionSeconds float64 `json:"positionSeconds"`
	IsPlaying       bool    `json:"isPlaying"`
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func generateCode() string {
	buf := make([]byte, 5)
	_, _ = rand.Read(buf)
	return strings.ToUpper(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf))
}

func (r *Repository) CreateRoom(ctx context.Context, hostUserID, mediaVersionID string) (*Room, error) {
	room := &Room{HostUserID: hostUserID, MediaVersionID: mediaVersionID, Code: generateCode()}
	err := r.db.QueryRow(ctx,
		`insert into rooms (code, host_user_id, media_version_id) values ($1, $2, $3)
		 returning id, position_seconds, is_playing`,
		room.Code, hostUserID, mediaVersionID,
	).Scan(&room.ID, &room.PositionSeconds, &room.IsPlaying)
	if err != nil {
		return nil, err
	}
	if err := r.AddMember(ctx, room.ID, hostUserID); err != nil {
		return nil, err
	}
	return room, nil
}

func (r *Repository) GetRoomByCode(ctx context.Context, code string) (*Room, error) {
	var room Room
	err := r.db.QueryRow(ctx,
		`select id, code, host_user_id, media_version_id, position_seconds, is_playing
		 from rooms where code = $1`, code,
	).Scan(&room.ID, &room.Code, &room.HostUserID, &room.MediaVersionID, &room.PositionSeconds, &room.IsPlaying)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &room, nil
}

func (r *Repository) AddMember(ctx context.Context, roomID, userID string) error {
	_, err := r.db.Exec(ctx,
		`insert into room_members (room_id, user_id) values ($1, $2)
		 on conflict do nothing`, roomID, userID)
	return err
}

func (r *Repository) UpdatePlaybackState(ctx context.Context, roomID string, position float64, isPlaying bool) error {
	_, err := r.db.Exec(ctx,
		`update rooms set position_seconds = $1, is_playing = $2, updated_at = now() where id = $3`,
		position, isPlaying, roomID,
	)
	return err
}

func (r *Repository) SaveWatchProgress(ctx context.Context, userID, mediaVersionID string, position float64) error {
	_, err := r.db.Exec(ctx,
		`insert into watch_progress (user_id, media_version_id, position_seconds)
		 values ($1, $2, $3)
		 on conflict (user_id, media_version_id)
		 do update set position_seconds = excluded.position_seconds, updated_at = now()`,
		userID, mediaVersionID, position,
	)
	return err
}
