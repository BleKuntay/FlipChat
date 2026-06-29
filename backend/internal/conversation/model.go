package conversation

import "time"

type Conversation struct {
	ID         string    `db:"id"           json:"id"`
	UserLowID  string    `db:"user_low_id"  json:"user_low_id"`
	UserHighID string    `db:"user_high_id" json:"user_high_id"`
	CreatedAt  time.Time `db:"created_at"   json:"created_at"`
}

type URIParams struct {
	ID string `uri:"id"`
}

type CreateRequest struct {
	TargetUserID string `json:"target_user_id"`
}

type Response struct {
	ConversationID     string     `db:"conversation_id"      json:"conversation_id"`
	Name               string     `db:"name"                 json:"name"`
	Username           string     `db:"username"             json:"username"`
	AvatarURL          *string    `db:"avatar_url"           json:"avatar_url"`
	LastMessagePreview *string    `db:"last_message_preview" json:"last_message_preview"`
	LastMessageAt      *time.Time `db:"last_message_at"      json:"last_message_at"`
}

type ListResponse struct {
	Data []Response `json:"data"`
}
