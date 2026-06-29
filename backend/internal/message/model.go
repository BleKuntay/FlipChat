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
}

type URIParams struct {
	ConversationID string `uri:"id"`
}

type ListQuery struct {
	Cursor string `query:"cursor"`
	Limit  int    `query:"limit"`
}

type SendRequest struct {
	Content   string  `json:"content"`
	ReplyToID *string `json:"reply_to_id"`
}

type Response struct {
	ID             string           `json:"id"`
	ConversationID string           `json:"conversation_id"`
	SenderID       string           `json:"sender_id"`
	Content        *string          `json:"content"`
	ReplyToID      *string          `json:"reply_to_id,omitempty"`
	Metadata       *json.RawMessage `json:"metadata,omitempty"`
	IsEdited       bool             `json:"is_edited"`
	CreatedAt      time.Time        `json:"created_at"`
}

type ListResponse struct {
	Data       []*Response `json:"data"`
	NextCursor *string     `json:"next_cursor"`
	HasMore    bool        `json:"has_more"`
}
