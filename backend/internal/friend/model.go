package friend

import "time"

// ------------------------------------------------------------------ //
// Status                                                               //
// ------------------------------------------------------------------ //

type Status string

const (
	StatusPending  Status = "pending"
	StatusAccepted Status = "accepted"

	StatusNone            Status = "none"
	StatusPendingSent     Status = "pending_sent"
	StatusPendingReceived Status = "pending_received"
)

// ------------------------------------------------------------------ //
// Domain model                                                         //
// ------------------------------------------------------------------ //

type Friend struct {
	UserLowID   string    `db:"user_low_id"`
	UserHighID  string    `db:"user_high_id"`
	RequesterID string    `db:"requester_id"`
	Status      Status    `db:"status"`
	CreatedAt   time.Time `db:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"`
}

// ------------------------------------------------------------------ //
// URI params                                                           //
// ------------------------------------------------------------------ //

type URIParams struct {
	ID string `params:"id"`
}

// ------------------------------------------------------------------ //
// Query params                                                         //
// ------------------------------------------------------------------ //

type ListQuery struct {
	Q      string `query:"q"`
	Cursor string `query:"cursor"`
	Limit  int    `query:"limit"`
}

type RequestListQuery struct {
	Direction string `query:"direction"`
	Cursor    string `query:"cursor"`
	Limit     int    `query:"limit"`
}

// ------------------------------------------------------------------ //
// Responses                                                            //
// ------------------------------------------------------------------ //

type Response struct {
	UserID      string    `db:"user_id"      json:"user_id"`
	Username    string    `db:"username"     json:"username"`
	FullName    string    `db:"name"         json:"full_name"`
	FriendSince time.Time `db:"friend_since" json:"friend_since"`
}

type PendingResponse struct {
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	FullName  string    `json:"full_name"`
	Direction string    `json:"direction"` // "sent" | "received"
	Status    Status    `json:"status"`    // "pending" | "accepted"
	CreatedAt time.Time `json:"created_at"`
}

type StatusResponse struct {
	Status Status `json:"status"`
}

// ------------------------------------------------------------------ //
// Paginated list responses                                             //
// ------------------------------------------------------------------ //

type ListResponse struct {
	Friends    []Response `json:"friends"`
	NextCursor *string    `json:"next_cursor"`
}

type RequestListResponse struct {
	Requests   []PendingResponse `json:"requests"`
	NextCursor *string           `json:"next_cursor"`
}

// ------------------------------------------------------------------ //
// Internal                                                             //
// ------------------------------------------------------------------ //

type Record struct {
	UserID      string    `db:"user_id"`
	Username    string    `db:"username"`
	FullName    string    `db:"name"`
	CreatedAt   time.Time `db:"created_at"`
	RequesterID string    `db:"requester_id"`
	Status      Status    `db:"status"`
}
