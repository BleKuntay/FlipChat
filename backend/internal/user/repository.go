package user

import (
	"database/sql"
	"errors"
	"fmt"
	"github.com/BleKuntay/FlipChat/backend/pkg/apperr"
	"github.com/jmoiron/sqlx"
)

type Repository struct {
	db *sqlx.DB
}

func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) FindByID(userID string) (*User, error) {
	var user User

	query := "SELECT * FROM users WHERE id = $1"
	if err := r.db.Get(&user, query, userID); err != nil {
		return nil, err
	}

	return &user, nil
}

func (r *Repository) UpdateProfile(u *User) (*UpdateProfileResponse, error) {
	query := `
		UPDATE users
		SET name = $1, username = $2, bio = $3, language = $4
		WHERE id = $5
		RETURNING id, name, username, bio, email, language, avatar_url, last_seen_at, created_at
	`

	res := &UpdateProfileResponse{}
	err := r.db.QueryRow(query, u.Name, u.Username, u.Bio, u.Language, u.ID).Scan(
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

func (r *Repository) UpdateEmail(userID string, request *UpdateEmailRequest) (*MeResponse, error) {
	query := `
        UPDATE users SET email = $1
        WHERE id = $2
        RETURNING id, name, username, bio, email, language, avatar_url, last_seen_at, created_at
    `

	res := &MeResponse{}
	err := r.db.QueryRow(query, request.NewEmail, userID).Scan(
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

func (r *Repository) UpdatePassword(userID, hashedPassword string) error {
	query := "UPDATE users SET password = $1 WHERE id = $2"

	if _, err := r.db.Exec(query, hashedPassword, userID); err != nil {
		return err
	}

	return nil
}

func (r *Repository) DeleteByID(userID string) error {
	query := "DELETE FROM users WHERE id = $1"

	if _, err := r.db.Exec(query, userID); err != nil {
		return err
	}

	return nil
}

func (r *Repository) Search(userID, q, cursor string, limit int) ([]*Summary, error) {
	args := []any{userID, "%" + q + "%"}

	query := `
        SELECT id, name, username, avatar_url, last_seen_at
        FROM users
        WHERE id <> $1 AND username ILIKE $2
    `

	if cursor != "" {
		args = append(args, cursor)
		query += fmt.Sprintf(` AND id > $%d`, len(args))
	}

	args = append(args, limit)
	query += fmt.Sprintf(` ORDER BY id ASC LIMIT $%d`, len(args))

	rows, err := r.db.Query(query, args...)
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
