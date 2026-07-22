package ws

import "encoding/json"

const (
	EventMessageNew      = "message.new"
	EventMessageEdited   = "message.edited"
	EventMessageDeleted  = "message.deleted"
	EventMessageRead     = "message.read"
	EventPresenceOnline  = "presence.online"
	EventPresenceOffline = "presence.offline"
	EventHeartbeat       = "heartbeat"
)

type Event struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type MessagePayload struct {
	ID             string  `json:"id"`
	ConversationID string  `json:"conversation_id"`
	SenderID       string  `json:"sender_id"`
	Content        *string `json:"content"`
	ReplyToID      *string `json:"reply_to_id"`
	IsEdited       bool    `json:"is_edited"`
	IsDeleted      bool    `json:"is_deleted"`
}

type MessageEditedPayload struct {
	ID             string `json:"id"`
	ConversationID string `json:"conversation_id"`
	Content        string `json:"content"`
}

type MessageDeletedPayload struct {
	ID             string `json:"id"`
	ConversationID string `json:"conversation_id"`
}

type MessageReadPayload struct {
	MessageID      string `json:"message_id"`
	ConversationID string `json:"conversation_id"`
	ReaderID       string `json:"reader_id"`
}

type PresencePayload struct {
	UserID string `json:"user_id"`
}

func NewEvent(eventType string, payload any) (Event, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return Event{}, err
	}
	return Event{Type: eventType, Payload: b}, nil
}
