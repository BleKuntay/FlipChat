package conversation

import (
	"context"
	"database/sql"
	"errors"
	"github.com/BleKuntay/FlipChat/backend/pkg/apperr"
	"github.com/jmoiron/sqlx"
)

type Repository struct {
	db *sqlx.DB
}

func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) FindAllByUserID(ctx context.Context, userID string) ([]Response, error) {
	q := `
		SELECT
			c.id AS conversation_id,
			u.name,
			u.username,
			u.avatar_url,
			m.content AS last_message_preview,
			m.created_at AS last_message_at
		FROM conversations c
		JOIN users u ON u.id = CASE
			WHEN c.user_low_id = $1 THEN c.user_high_id
			ELSE c.user_low_id
		END
		LEFT JOIN LATERAL(
		    SELECT content, created_at
			FROM messages
		    WHERE conversation_id = c.id
		    ORDER BY created_at DESC
		    LIMIT 1
		) m ON true
		WHERE user_low_id = $1 OR user_high_id = $1
		ORDER BY m.created_at DESC NULLS LAST, c.created_at DESC
	`

	var results []Response
	if err := r.db.SelectContext(ctx, &results, q, userID); err != nil {
		return nil, err
	}

	return results, nil
}

func (r *Repository) FindByID(ctx context.Context, requesterID, conversationID string) (*Response, error) {
	q := `
		SELECT
			c.id AS conversation_id,
			u.name,
			u.username,
			u.avatar_url,
			m.content AS last_message_preview,
			m.created_at AS last_message_at
		FROM conversations c
		JOIN users u ON u.id = CASE
			WHEN c.user_low_id = $1 THEN c.user_high_id
			ELSE c.user_low_id
		END
		LEFT JOIN LATERAL(
		    SELECT content, created_at
			FROM messages
		    WHERE conversation_id = c.id
			ORDER BY created_at DESC
			LIMIT 1
		) m ON true
		WHERE c.id = $2 AND (c.user_low_id = $1 OR c.user_high_id = $1)
	`

	var response Response
	if err := r.db.GetContext(ctx, &response, q, requesterID, conversationID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperr.ErrNotFound
		}
		return nil, err
	}

	return &response, nil
}

func (r *Repository) FindByPair(ctx context.Context, lowID, highID string) (*Conversation, error) {
	q := `
		SELECT id, user_low_id, user_high_id, created_at
		FROM conversations
		WHERE user_low_id = $1 AND user_high_id = $2
	`

	var conversation Conversation
	if err := r.db.GetContext(ctx, &conversation, q, lowID, highID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}

		return nil, err
	}

	return &conversation, nil
}

func (r *Repository) Create(ctx context.Context, requesterID, lowID, highID string) (*Response, error) {
	q := `
		WITH upsert AS (
			INSERT INTO conversations (user_low_id, user_high_id)
			VALUES ($2, $3)
			RETURNING id, user_low_id, user_high_id
		)
		SELECT
			upsert.id AS conversation_id,
			u.name,
			u.username,
			u.avatar_url,
			NULL::TEXT        AS last_message_preview,
			NULL::TIMESTAMPTZ AS last_message_at
		FROM upsert
		JOIN users u ON u.id = CASE
			WHEN upsert.user_low_id = $1 THEN upsert.user_high_id
			ELSE upsert.user_low_id
		END
	`

	var response Response
	if err := r.db.GetContext(ctx, &response, q, requesterID, lowID, highID); err != nil {
		return nil, err
	}

	return &response, nil
}

func (r *Repository) GetParticipants(ctx context.Context, conversationID string) (userLowID, userHighID string, err error) {
	const q = "SELECT user_low_id, user_high_id FROM conversations WHERE id = $1"

	var conversation Conversation
	if err := r.db.GetContext(ctx, &conversation, q, conversationID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", "", apperr.ErrNotFound
		}

		return "", "", err
	}

	return conversation.UserLowID, conversation.UserHighID, nil
}
