package friend

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
)

type Repository struct {
	db *sqlx.DB
}

func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) FindAllRequests(ctx context.Context, userID string, query RequestListQuery) ([]Record, error) {
	q := `
		SELECT
			u.id AS user_id,
			u.username,
			u.name,
			f.created_at,
			f.requester_id,
			f.status
		FROM (
			SELECT user_high_id AS other_id, created_at, requester_id, status
			FROM friends
			WHERE user_low_id = $1 AND status = 'pending'
			UNION ALL
			SELECT user_low_id AS other_id, created_at, requester_id, status
			FROM friends
			WHERE user_high_id = $1 AND status = 'pending'
		) f
		JOIN users u ON u.id = f.other_id
		WHERE ($2::uuid IS NULL OR u.id > $2::uuid)
		ORDER BY u.id
		LIMIT $3
	`

	cursor := stringToNullable(query.Cursor)
	var records []Record
	if err := r.db.SelectContext(ctx, &records, q, userID, cursor, query.Limit+1); err != nil {
		return nil, err
	}

	return records, nil
}

func (r *Repository) FindAll(ctx context.Context, userID string, query ListQuery) ([]Response, error) {
	q := `
		SELECT
			u.id AS user_id,
			u.username,
			u.name,
			f.friend_since
		FROM (
			SELECT user_high_id AS friend_id, created_at AS friend_since
			FROM friends
			WHERE user_low_id = $1 AND status = 'accepted'
			UNION ALL
			SELECT user_low_id AS friend_id, created_at AS friend_since
			FROM friends
			WHERE user_high_id = $1 AND status = 'accepted'
		) f
		JOIN users u ON u.id = f.friend_id
		WHERE ($2 = '' OR u.username ILIKE '%' || $2 || '%' OR u.name ILIKE '%' || $2 || '%')
		AND ($3::uuid IS NULL OR u.id > $3::uuid)
		ORDER BY u.id
		LIMIT $4
	`

	cursor := stringToNullable(query.Cursor)
	var friends []Response
	if err := r.db.SelectContext(ctx, &friends, q, userID, query.Q, cursor, query.Limit+1); err != nil {
		return nil, err
	}

	return friends, nil
}

func (r *Repository) ExistsByUserID(ctx context.Context, userID string) (bool, error) {
	q := "SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)"

	var exists bool
	if err := r.db.GetContext(ctx, &exists, q, userID); err != nil {
		return false, err
	}

	return exists, nil
}

func (r *Repository) FindByPair(ctx context.Context, lowID, highID string) (*Friend, error) {
	q := `
		SELECT user_low_id, user_high_id, requester_id, status, created_at, updated_at
		FROM friends
		WHERE user_low_id = $1 AND user_high_id = $2
	`

	var friend Friend
	if err := r.db.GetContext(ctx, &friend, q, lowID, highID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &friend, nil
}

func (r *Repository) UpsertFriend(ctx context.Context, lowID, highID, requesterID string) (*Record, error) {
	q := `
		WITH upsert AS (
			INSERT INTO friends (user_low_id, user_high_id, requester_id)
			VALUES ($1, $2, $3)
			ON CONFLICT (user_low_id, user_high_id) DO UPDATE
				SET status     = 'accepted',
				    updated_at = now()
				WHERE friends.status        = 'pending'
				  AND friends.requester_id != EXCLUDED.requester_id
			RETURNING user_low_id, user_high_id, requester_id, status, created_at
		)
		SELECT
			u.id AS user_id,
			u.username,
			u.name,
			upsert.created_at,
			upsert.requester_id,
			upsert.status
		FROM upsert
		JOIN users u ON u.id = CASE
			WHEN upsert.requester_id = upsert.user_low_id THEN upsert.user_high_id
			ELSE upsert.user_low_id
		END
	`

	var record Record
	if err := r.db.GetContext(ctx, &record, q, lowID, highID, requesterID); err != nil {
		return nil, err
	}

	return &record, nil
}

func (r *Repository) DeleteByPair(ctx context.Context, lowID, highID string) error {
	q := "DELETE FROM friends WHERE user_low_id = $1 AND user_high_id = $2"

	if _, err := r.db.ExecContext(ctx, q, lowID, highID); err != nil {
		return err
	}

	return nil
}

func stringToNullable(s string) *string {
	if s == "" {
		return nil
	}

	return &s
}
