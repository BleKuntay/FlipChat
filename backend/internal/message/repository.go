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
		SELECT 
		    id, conversation_id, sender_id, content, reply_to_id, metadata, 
       		is_edited, created_at, updated_at, read_at, deleted_at, deleted_by
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
        SELECT 
            id, conversation_id, sender_id, content, reply_to_id, metadata, 
            is_edited, created_at, updated_at, read_at, deleted_at, deleted_by
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

func (r *Repository) EditMessage(ctx context.Context, message *Message) error {
	q := `
		UPDATE messages
		SET content = $1, is_edited = true, updated_at = $2
		WHERE id = $3
	`

	res, err := r.db.ExecContext(ctx, q, message.Content, message.UpdatedAt, message.ID)
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

func (r *Repository) Delete(ctx context.Context, message *Message) error {
	q := `
		UPDATE messages
		SET content = NULL, deleted_at = $1, deleted_by = $2
		WHERE id = $3
	`

	res, err := r.db.ExecContext(ctx, q, message.DeletedAt, message.DeletedBy, message.ID)
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

func (r *Repository) MarkAsRead(ctx context.Context, message *Message) error {
	q := "UPDATE messages SET read_at = $1 WHERE id = $2"

	res, err := r.db.ExecContext(ctx, q, message.ReadAt, message.ID)
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

func stringToNullable(s string) *string {
	if s == "" {
		return nil
	}

	return &s
}
