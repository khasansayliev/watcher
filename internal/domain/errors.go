package domain

import "errors"

var (
	ErrNotFound     = errors.New("topilmadi")
	ErrForbidden    = errors.New("bu amalga ruxsat yo'q")
	ErrUnauthorized = errors.New("tizimga kirish kerak")
)

// ValidationError — foydalanuvchi kiritgan ma'lumot biznes qoidalariga mos emas.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

func NewValidationError(msg string) error { return &ValidationError{Msg: msg} }
