package block

import "time"

type Block struct {
	BlockerID string    `db:"blocker_id" json:"blocker_id"`
	BlockedID string    `db:"blocked_id" json:"blocked_id"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

type URIParams struct {
	ID string `params:"id"`
}

type ListQuery struct {
	Cursor string `query:"cursor"`
	Limit  int    `query:"limit"`
}

type Request struct {
	BlockerID string
	BlockedID string
}

type Response struct {
	BlockerID string `db:"blocker_id" json:"blocker_id"`
	BlockedID string `db:"blocked_id" json:"blocked_id"`
}

type BlockedSummary struct {
	UserID    string  `db:"user_id"    json:"user_id"`
	Name      string  `db:"name"       json:"name"`
	Username  string  `db:"username"   json:"username"`
	AvatarUrl *string `db:"avatar_url" json:"avatar_url"`
}

type ListResponse struct {
	Data       []BlockedSummary `json:"data"`
	NextCursor string           `json:"next_cursor,omitempty"`
}
