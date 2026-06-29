package block

import (
	"context"
	"database/sql"
	"github.com/jmoiron/sqlx"
)

type dbContext interface {
	GetContext(ctx context.Context, dest any, query string, args ...any) error
	SelectContext(ctx context.Context, dest any, query string, args ...any) error
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type Repository struct {
	db  *sqlx.DB
	ext dbContext
}

func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{db: db, ext: db}
}

func (r *Repository) WithTx(tx *sqlx.Tx) *Repository {
	return &Repository{db: r.db, ext: tx}
}

func (r *Repository) BlockUserAtomic(ctx context.Context, request Request, low, high string) (*Response, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		"DELETE FROM friends WHERE user_low_id = $1 AND user_high_id = $2",
		low, high,
	); err != nil {
		return nil, err
	}

	var response Response
	if err := tx.GetContext(ctx, &response,
		"INSERT INTO blocks (blocker_id, blocked_id) VALUES ($1, $2) RETURNING blocker_id, blocked_id",
		request.BlockerID, request.BlockedID,
	); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &response, nil
}

func (r *Repository) UnblockUser(ctx context.Context, request Request) error {
	q := "DELETE FROM blocks WHERE blocker_id = $1 AND blocked_id = $2"

	if _, err := r.ext.ExecContext(ctx, q, request.BlockerID, request.BlockedID); err != nil {
		return err
	}

	return nil
}

func (r *Repository) GetBlockList(ctx context.Context, blockerID string, query ListQuery) ([]BlockedSummary, error) {
	q := `
        SELECT
            b.blocked_id AS user_id,
            u.name,
            u.username,
            u.avatar_url
        FROM blocks b
        JOIN users u ON b.blocked_id = u.id
        WHERE blocker_id = $1 AND ($2::text IS NULL OR u.id > $2::text)
		ORDER BY u.id
		LIMIT $3
    `

	cursor := stringToNullable(query.Cursor)
	var records []BlockedSummary
	if err := r.ext.SelectContext(ctx, &records, q, blockerID, cursor, query.Limit); err != nil {
		return nil, err
	}

	return records, nil
}

func (r *Repository) IsBlockedEitherWay(ctx context.Context, a, b string) (bool, error) {
	q := `
		SELECT EXISTS(
			SELECT 1 FROM blocks
			WHERE (blocker_id = $1 AND blocked_id = $2)
			   OR (blocker_id = $2 AND blocked_id = $1)
		)
	`

	var exists bool
	if err := r.ext.GetContext(ctx, &exists, q, a, b); err != nil {
		return false, err
	}

	return exists, nil
}

func stringToNullable(s string) *string {
	if s == "" {
		return nil
	}

	return &s
}
