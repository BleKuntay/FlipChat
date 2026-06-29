package auth

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
)

type Repository struct {
	db *sqlx.DB
}

func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CreateUser(ctx context.Context, u *User) (*User, error) {
	query := `
		INSERT INTO users (name, username, email, password, language)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING *
	`

	var user User
	if err := r.db.QueryRowxContext(ctx, query, u.Name, u.Username, u.Email, u.Password, u.Language).StructScan(&user); err != nil {
		return nil, err
	}

	return &user, nil
}

func (r *Repository) FindUserByEmail(ctx context.Context, email string) (*User, error) {
	query := "SELECT * FROM users WHERE email = $1"

	var user User
	if err := r.db.QueryRowxContext(ctx, query, email).StructScan(&user); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &user, nil
}

func (r *Repository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	query := "SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)"

	var exists bool
	if err := r.db.QueryRowContext(ctx, query, email).Scan(&exists); err != nil {
		return false, err
	}

	return exists, nil
}

func (r *Repository) ExistsByUsername(ctx context.Context, username string) (bool, error) {
	query := "SELECT EXISTS(SELECT 1 FROM users WHERE username = $1)"

	var exists bool
	if err := r.db.QueryRowContext(ctx, query, username).Scan(&exists); err != nil {
		return false, err
	}

	return exists, nil
}

func (r *Repository) SaveRefreshToken(ctx context.Context, token RefreshToken) error {
	query := `
		INSERT INTO refresh_tokens (user_id, token, expires_at)
		VALUES ($1, $2, $3)
	`

	if _, err := r.db.ExecContext(ctx, query, token.UserID, token.Token, token.ExpiresAt); err != nil {
		return err
	}

	return nil
}

func (r *Repository) DeleteTokenByToken(ctx context.Context, token string) error {
	query := "DELETE FROM refresh_tokens WHERE token = $1"

	if _, err := r.db.ExecContext(ctx, query, token); err != nil {
		return err
	}

	return nil
}

func (r *Repository) DeleteTokenByUserID(ctx context.Context, userID string) error {
	query := "DELETE FROM refresh_tokens WHERE user_id = $1"

	if _, err := r.db.ExecContext(ctx, query, userID); err != nil {
		return err
	}

	return nil
}

func (r *Repository) FindTokenByToken(ctx context.Context, token string) (*RefreshToken, error) {
	query := "SELECT * FROM refresh_tokens WHERE token = $1"

	var rt RefreshToken
	if err := r.db.QueryRowxContext(ctx, query, token).StructScan(&rt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &rt, nil
}

func (r *Repository) RotateRefreshToken(ctx context.Context, oldToken, newToken string, expiresAt time.Time) error {
	query := `
		UPDATE refresh_tokens
		SET token = $1, expires_at = $2
		WHERE token = $3
	`

	if _, err := r.db.ExecContext(ctx, query, newToken, expiresAt, oldToken); err != nil {
		return err
	}

	return nil
}
