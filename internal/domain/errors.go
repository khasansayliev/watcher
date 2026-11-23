package domain

import "errors"

// ErrNotFound — kuzatuv topilmadi.
var ErrNotFound = errors.New("kuzatuv topilmadi")

// ValidationError — foydalanuvchi kiritgan ma'lumot biznes qoidalariga mos emas.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

func NewValidationError(msg string) error { return &ValidationError{Msg: msg} }
