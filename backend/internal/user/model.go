package user

import "time"

type User struct {
	ID         string     `db:"id"           json:"id"`
	Name       string     `db:"name"         json:"name"`
	Username   string     `db:"username"     json:"username"`
	Bio        *string    `db:"bio"          json:"bio"`
	Email      string     `db:"email"        json:"email"`
	Password   string     `db:"password"     json:"-"`
	Language   string     `db:"language"     json:"language"`
	AvatarURL  *string    `db:"avatar_url"   json:"avatar_url,omitempty"`
	LastSeenAt *time.Time `db:"last_seen_at" json:"last_seen_at"`
	CreatedAt  time.Time  `db:"created_at"   json:"created_at"`
	UpdatedAt  time.Time  `db:"updated_at"   json:"updated_at"`
}

// ----- Request -----

type UpdateProfileRequest struct {
	Name     string `json:"name"`
	Username string `json:"username"`
	Bio      string `json:"bio"`
	Language string `json:"language"`
}

type UpdateEmailRequest struct {
	NewEmail        string `json:"new_email"`
	CurrentPassword string `json:"current_password"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
	ConfirmPassword string `json:"confirm_password"`
}

type SearchQuery struct {
	Q      string `query:"q"`
	Limit  int    `query:"limit"`
	Cursor string `query:"cursor"`
}

type GetUserURI struct {
	ID string `uri:"id"`
}

// ----- Response -----

type Summary struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Username   string     `json:"username"`
	AvatarURL  *string    `json:"avatar_url"`
	LastSeenAt *time.Time `json:"last_seen_at"`
}

type Response struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Username   string     `json:"username"`
	Bio        *string    `json:"bio"`
	Language   string     `json:"language"`
	AvatarURL  *string    `json:"avatar_url"`
	LastSeenAt *time.Time `json:"last_seen_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

type MeResponse struct {
	Response
	Email string `json:"email"`
}

type SearchResponse struct {
	Data       []*Summary `json:"data"`
	NextCursor *string    `json:"next_cursor"`
}

type UpdateProfileResponse = MeResponse

type UpdateEmailResponse = MeResponse
