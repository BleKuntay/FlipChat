package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/BleKuntay/FlipChat/backend/pkg/apperr"
	"github.com/redis/go-redis/v9"
	"time"

	"github.com/jmoiron/sqlx"
)

type Repository struct {
	db  *sqlx.DB
	rdb *redis.Client
	ttl time.Duration
}

func NewRepository(db *sqlx.DB, rdb *redis.Client, ttl time.Duration) *Repository {
	return &Repository{db: db, rdb: rdb, ttl: ttl}
}

func tokenKey(token string) string {
	return fmt.Sprintf("refresh_token:%s", token)
}

func userTokensKey(userID string) string {
	return fmt.Sprintf("user_refresh_tokens:%s", userID)
}

// ── User methods (PostgreSQL) ─────────────────────────────────────────────────

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

// ── Token methods (Redis) ─────────────────────────────────────────────────────

func (r *Repository) SaveRefreshToken(ctx context.Context, token RefreshToken) error {
	ttl := time.Until(token.ExpiresAt)
	if ttl <= 0 {
		return apperr.ErrBadRequest
	}

	pipe := r.rdb.Pipeline()
	pipe.Set(ctx, tokenKey(token.Token), token.UserID, ttl)
	pipe.SAdd(ctx, userTokensKey(token.UserID), token.Token)
	pipe.Expire(ctx, tokenKey(token.Token), ttl)

	_, err := pipe.Exec(ctx)
	return err
}

func (r *Repository) FindTokenByToken(ctx context.Context, token string) (*RefreshToken, error) {
	userID, err := r.rdb.Get(ctx, tokenKey(token)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}
		return nil, err
	}

	ttl, err := r.rdb.TTL(ctx, tokenKey(token)).Result()
	if err != nil {
		return nil, err
	}

	return &RefreshToken{
		UserID:    userID,
		Token:     token,
		ExpiresAt: time.Now().Add(ttl),
	}, nil
}

func (r *Repository) DeleteTokenByToken(ctx context.Context, token string) error {
	userID, err := r.rdb.Get(ctx, tokenKey(token)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil
		}
		return err
	}

	pipe := r.rdb.Pipeline()
	pipe.Del(ctx, tokenKey(token))
	pipe.SRem(ctx, userTokensKey(userID), token)

	_, err = pipe.Exec(ctx)
	return err
}

func (r *Repository) DeleteTokenByUserID(ctx context.Context, userID string) error {
	tokens, err := r.rdb.SMembers(ctx, userTokensKey(userID)).Result()
	if err != nil {
		return err
	}
	if len(tokens) == 0 {
		return nil
	}

	pipe := r.rdb.Pipeline()
	for _, token := range tokens {
		pipe.Del(ctx, tokenKey(token))
	}
	pipe.Del(ctx, userTokensKey(userID))

	_, err = pipe.Exec(ctx)
	return err
}

func (r *Repository) RotateRefreshToken(ctx context.Context, oldToken, newToken string, expiresAt time.Time) error {
	userID, err := r.rdb.Get(ctx, tokenKey(oldToken)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return apperr.ErrNotFound
		}
		return err
	}

	ttl := time.Until(expiresAt)
	if ttl <= 0 {
		return apperr.ErrBadRequest
	}

	pipe := r.rdb.Pipeline()

	pipe.Del(ctx, tokenKey(oldToken))
	pipe.SRem(ctx, userTokensKey(userID), oldToken)

	pipe.Set(ctx, tokenKey(newToken), userID, ttl)
	pipe.SAdd(ctx, userTokensKey(userID), newToken)
	pipe.Expire(ctx, userTokensKey(userID), ttl)

	_, err = pipe.Exec(ctx)
	return err
}
