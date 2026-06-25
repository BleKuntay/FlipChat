package auth

import (
	"database/sql"
	"errors"
	"github.com/jmoiron/sqlx"
	"time"
)

type Repository struct {
	db *sqlx.DB
}

func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CreateUser(u *User) (*User, error) {
	query := `
		INSERT INTO users (name, username, email, password, language)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING *
	`

	var user User
	if err := r.db.QueryRowx(query, u.Name, u.Username, u.Email, u.Password, u.Language).StructScan(&user); err != nil {
		return nil, err
	}

	return &user, nil
}

func (r *Repository) FindUserByEmail(email string) (*User, error) {
	query := "SELECT * FROM users WHERE email = $1"

	var user User
	if err := r.db.QueryRowx(query, email).StructScan(&user); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &user, nil
}

func (r *Repository) ExistsByEmail(email string) (bool, error) {
	query := "SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)"

	var exists bool
	if err := r.db.QueryRow(query, email).Scan(&exists); err != nil {
		return false, err
	}

	return exists, nil
}

func (r *Repository) ExistsByUsername(username string) (bool, error) {
	query := "SELECT EXISTS(SELECT 1 FROM users WHERE username = $1)"

	var exists bool
	if err := r.db.QueryRow(query, username).Scan(&exists); err != nil {
		return false, err
	}

	return exists, nil
}

func (r *Repository) SaveRefreshToken(token RefreshToken) error {
	query := `
		INSERT INTO refresh_tokens (user_id, token, expires_at)
		VALUES ($1, $2, $3)
	`

	_, err := r.db.Exec(query, token.UserID, token.Token, token.ExpiresAt)
	if err != nil {
		return err
	}

	return nil
}

func (r *Repository) DeleteTokenByToken(token string) error {
	query := "DELETE FROM refresh_tokens WHERE token = $1"

	_, err := r.db.Exec(query, token)
	if err != nil {
		return err
	}

	return nil
}

func (r *Repository) DeleteTokenByUserID(token string) error {
	query := "DELETE FROM refresh_tokens WHERE user_id = $1"

	_, err := r.db.Exec(query, token)
	if err != nil {
		return err
	}

	return nil
}

func (r *Repository) FindTokenByToken(token string) (*RefreshToken, error) {
	query := "SELECT * FROM refresh_tokens WHERE token = $1"

	var rt RefreshToken
	if err := r.db.QueryRowx(query, token).StructScan(&rt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &rt, nil
}

func (r *Repository) RotateRefreshToken(oldToken, newToken string, expiresAt time.Time) error {
	query := `
		UPDATE refresh_tokens
		SET token = $1, expires_at = $2
		WHERE token = $3
	`

	_, err := r.db.Exec(query, newToken, expiresAt, oldToken)
	if err != nil {
		return err
	}

	return nil
}
