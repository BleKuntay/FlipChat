package user

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/BleKuntay/FlipChat/backend/pkg/apperr"
	"github.com/jmoiron/sqlx"
)

type Repository struct {
	db *sqlx.DB
}

func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) FindByID(ctx context.Context, userID string) (*User, error) {
	var user User

	query := "SELECT * FROM users WHERE id = $1"
	if err := r.db.GetContext(ctx, &user, query, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperr.ErrNotFound
		}
		return nil, err
	}

	return &user, nil
}

func (r *Repository) UpdateProfile(ctx context.Context, u *User) (*UpdateProfileResponse, error) {
	query := `
		UPDATE users
		SET name = $1, username = $2, bio = $3, language = $4
		WHERE id = $5
		RETURNING id, name, username, bio, email, language, avatar_url, last_seen_at, created_at
	`

	res := &UpdateProfileResponse{}
	err := r.db.QueryRowContext(ctx, query, u.Name, u.Username, u.Bio, u.Language, u.ID).Scan(
		&res.ID,
		&res.Name,
		&res.Username,
		&res.Bio,
		&res.Email,
		&res.Language,
		&res.AvatarURL,
		&res.LastSeenAt,
		&res.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (r *Repository) UpdateEmail(ctx context.Context, userID string, request *UpdateEmailRequest) (*MeResponse, error) {
	query := `
		UPDATE users SET email = $1
		WHERE id = $2
		RETURNING id, name, username, bio, email, language, avatar_url, last_seen_at, created_at
	`

	res := &MeResponse{}
	err := r.db.QueryRowContext(ctx, query, request.NewEmail, userID).Scan(
		&res.ID,
		&res.Name,
		&res.Username,
		&res.Bio,
		&res.Email,
		&res.Language,
		&res.AvatarURL,
		&res.LastSeenAt,
		&res.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (r *Repository) UpdatePassword(ctx context.Context, userID, hashedPassword string) error {
	query := "UPDATE users SET password = $1 WHERE id = $2"

	res, err := r.db.ExecContext(ctx, query, hashedPassword, userID)
	if err != nil {
		return err
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return apperr.ErrNotFound
	}

	return nil
}

func (r *Repository) DeleteByID(ctx context.Context, userID string) error {
	query := "DELETE FROM users WHERE id = $1"

	res, err := r.db.ExecContext(ctx, query, userID)
	if err != nil {
		return err
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return apperr.ErrNotFound
	}

	return nil
}

func (r *Repository) Search(ctx context.Context, userID, q, cursor string, limit int) ([]*Summary, error) {
	args := []any{userID, "%" + q + "%"}

	query := `
		SELECT u.id, u.name, u.username, u.avatar_url, u.last_seen_at
		FROM users u
		WHERE u.id <> $1
		  AND u.username ILIKE $2
		  AND NOT EXISTS (
			  SELECT 1 FROM blocks
			  WHERE blocker_id = u.id AND blocked_id = $1
		  )
	`

	if cursor != "" {
		args = append(args, cursor)
		query += fmt.Sprintf(` AND u.id > $%d`, len(args))
	}

	args = append(args, limit)
	query += fmt.Sprintf(` ORDER BY u.id ASC LIMIT $%d`, len(args))

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summaries []*Summary
	for rows.Next() {
		s := &Summary{}
		if err := rows.Scan(&s.ID, &s.Name, &s.Username, &s.AvatarURL, &s.LastSeenAt); err != nil {
			return nil, err
		}
		summaries = append(summaries, s)
	}

	return summaries, nil
}

func (r *Repository) UpdateLastSeen(ctx context.Context, userID string, t time.Time) error {
	q := "UPDATE users SET last_seen_at = $1 WHERE id = $2"

	res, err := r.db.ExecContext(ctx, q, t, userID)
	if err != nil {
		return err
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return apperr.ErrNotFound
	}

	return nil
}
