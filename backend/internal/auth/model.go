package auth

import "time"

type RefreshToken struct {
	ID        string    `db:"id" json:"id"`
	UserID    string    `db:"user_id" json:"user_id"`
	Token     string    `db:"token" json:"token"`
	ExpiresAt time.Time `db:"expires_at" json:"expires_at"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

type LoginRequest struct {
	Email    string `db:"email" json:"email"`
	Password string `db:"password" json:"password"`
}

type RegisterRequest struct {
	Name     string  `json:"name"`
	Username string  `json:"username"`
	Email    string  `json:"email"`
	Password string  `json:"password"`
	Language *string `json:"language"`
}

type Response struct {
	AccessToken string       `json:"access_token"`
	User        UserResponse `json:"user"`
}

type User struct {
	ID         string     `db:"id"`
	Name       string     `db:"name"`
	Username   string     `db:"username"`
	Bio        *string    `db:"bio"`
	Email      string     `db:"email"`
	Password   string     `db:"password"`
	Language   string     `db:"language"`
	AvatarURL  *string    `db:"avatar_url"`
	LastSeenAt *time.Time `db:"last_seen_at"`
	CreatedAt  time.Time  `db:"created_at"`
	UpdatedAt  time.Time  `db:"updated_at"`
}

type UserResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Username  string    `json:"username"`
	Bio       *string   `json:"bio"`
	Email     string    `json:"email"`
	Language  string    `json:"language"`
	AvatarURL *string   `json:"avatar_url"`
	CreatedAt time.Time `json:"created_at"`
}
