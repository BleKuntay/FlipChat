package user

import "errors"

var (
	ErrInvalidPassword  = errors.New("invalid password")
	ErrPasswordMismatch = errors.New("password mismatch")
	ErrUserNotUpdated   = errors.New("user not updated")
)
