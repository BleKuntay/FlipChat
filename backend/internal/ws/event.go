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

type PresencePayload struct {
	UserID string `json:"user_id"`
}
