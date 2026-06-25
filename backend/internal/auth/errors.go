package auth

import "errors"

var (
	ErrEmailAlreadyInUse    = errors.New("email already in use")
	ErrUsernameAlreadyTaken = errors.New("username already taken")
	ErrInvalidCredentials   = errors.New("invalid credentials")
)
