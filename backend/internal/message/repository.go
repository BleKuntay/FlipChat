package message

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

func (r *Repository) Create(ctx context.Context, message *Message) error {
	q := `
		INSERT INTO messages (id, conversation_id, sender_id, content, reply_to_id, metadata, is_edited, created_at)
		VALUES (:id, :conversation_id, :sender_id, :content, :reply_to_id, :metadata, :is_edited, :created_at)
	`

	_, err := r.db.NamedExecContext(ctx, q, message)
	return err
}

func (r *Repository) GetByID(ctx context.Context, messageID string) (*Message, error) {
	q := `
		SELECT id, conversation_id, sender_id, content, reply_to_id, metadata, is_edited, created_at
		FROM messages
		WHERE id = $1
	`

	message := new(Message)
	if err := r.db.GetContext(ctx, message, q, messageID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperr.ErrNotFound
		}

		return nil, err
	}

	return message, nil
}

func (r *Repository) ListByConversation(ctx context.Context, conversationID, cursor string, limit int) ([]*Message, error) {
	q := `
        SELECT id, conversation_id, sender_id, content, reply_to_id, metadata, is_edited, created_at
		FROM messages
        WHERE conversation_id = $1 AND ($2::uuid IS NULL OR id < $2::uuid)
		ORDER BY id DESC
        LIMIT $3
	`

	var messages []*Message
	if err := r.db.SelectContext(ctx, &messages, q, conversationID, stringToNullable(cursor), limit); err != nil {
		return nil, err
	}

	return messages, nil
}

func stringToNullable(s string) *string {
	if s == "" {
		return nil
	}

	return &s
}
