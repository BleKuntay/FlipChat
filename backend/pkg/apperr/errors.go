package apperr

import "errors"

var (
	ErrNotFound        = errors.New("not found")
	ErrConflict        = errors.New("conflict")
	ErrForbidden       = errors.New("forbidden")
	ErrBadRequest      = errors.New("bad request")
	ErrUnauthorized    = errors.New("unauthorized")
	ErrUnsupportedMIME = errors.New("unsupported file type: only JPEG, PNG, GIF, WebP allowed")
	ErrFileTooLarge    = errors.New("file too large: maximum 5MB")
)
