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
			f.requester_id
		FROM (
			SELECT user_high_id AS other_id, created_at, requester_id
			FROM friends
			WHERE user_low_id = $1 AND status = 'pending'
			UNION ALL
			SELECT user_low_id AS other_id, created_at, requester_id
			FROM friends
			WHERE user_high_id = $1 AND status = 'pending'
		) f
		JOIN users u ON u.id = f.other_id
		WHERE ($2 = '' OR u.id > $2)
		ORDER BY u.id
		LIMIT $3
	`

	var records []Record
	if err := r.db.SelectContext(ctx, &records, q, userID, query.Cursor, query.Limit+1); err != nil {
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
		AND ($3 = '' OR u.id > $3)
		ORDER BY u.id
		LIMIT $4
	`

	var friends []Response
	if err := r.db.SelectContext(ctx, &friends, q, userID, query.Q, query.Cursor, query.Limit+1); err != nil {
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

func (r *Repository) AddFriend(ctx context.Context, lowID, highID, requesterID string) (*Record, error) {
	q := `
        WITH inserted AS (
            INSERT INTO friends (user_low_id, user_high_id, requester_id)
            VALUES ($1, $2, $3)
            RETURNING user_low_id, user_high_id, requester_id, created_at
        )
        SELECT
            u.id AS user_id,
            u.username,
            u.name,
            i.created_at,
            i.requester_id
        FROM inserted i
        JOIN users u ON u.id = CASE
            WHEN i.requester_id = i.user_low_id THEN i.user_high_id
            ELSE i.user_low_id
        END
    `

	var record Record
	if err := r.db.GetContext(ctx, &record, q, lowID, highID, requesterID); err != nil {
		return nil, err
	}

	return &record, nil
}

func (r *Repository) AcceptFriend(ctx context.Context, lowID, highID string) (*Record, error) {
	q := `
        WITH updated AS (
            UPDATE friends
            SET status = 'accepted'
            WHERE user_low_id = $1 AND user_high_id = $2
            RETURNING requester_id, created_at
        )
        SELECT
            u.id AS user_id,
            u.username,
            u.name,
            upd.created_at,
            upd.requester_id
        FROM updated upd
        JOIN users u ON u.id = upd.requester_id
    `

	var record Record
	if err := r.db.GetContext(ctx, &record, q, lowID, highID); err != nil {
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
