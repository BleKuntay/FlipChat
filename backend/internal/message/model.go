package message

import (
	"encoding/json"
	"time"
)

type Message struct {
	ID             string           `db:"id"`
	ConversationID string           `db:"conversation_id"`
	SenderID       string           `db:"sender_id"`
	Content        *string          `db:"content"`
	ReplyToID      *string          `db:"reply_to_id"`
	Metadata       *json.RawMessage `db:"metadata"`
	IsEdited       bool             `db:"is_edited"`
	CreatedAt      time.Time        `db:"created_at"`
	UpdatedAt      *time.Time       `db:"updated_at"`
	ReadAt         *time.Time       `db:"read_at"`
	DeletedAt      *time.Time       `db:"deleted_at"`
	DeletedBy      *string          `db:"deleted_by"`
}

type URIParams struct {
	ConversationID string `uri:"id"`
	MessageID      string `uri:"msg_id"`
}

type ListQuery struct {
	Cursor string `query:"cursor"`
	Limit  int    `query:"limit"`
}

type SendRequest struct {
	Content      string  `json:"content"`
	ReplyToID    *string `json:"reply_to_id"`
	AttachmentID *string `json:"attachment_id"`
	// Filename, MIMEType, Size intentionally absent — metadata is read from
	// the server-side upload record, never trusted from the client.
}

type EditRequest struct {
	Content string `json:"content"`
}

type Response struct {
	ID             string           `json:"id"`
	ConversationID string           `json:"conversation_id"`
	SenderID       string           `json:"sender_id"`
	Content        *string          `json:"content"`
	ReplyToID      *string          `json:"reply_to_id,omitempty"`
	Metadata       *json.RawMessage `json:"metadata,omitempty"`
	IsEdited       bool             `json:"is_edited"`
	IsDeleted      bool             `json:"is_deleted"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      *time.Time       `json:"updated_at,omitempty"`
	ReadAt         *time.Time       `json:"read_at,omitempty"`
}

type ListResponse struct {
	Data       []*Response `json:"data"`
	NextCursor *string     `json:"next_cursor"`
	HasMore    bool        `json:"has_more"`
}
