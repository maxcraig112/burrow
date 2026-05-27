package exchange

import "fmt"

// Code identifies the category of an exchange error.
type Code string

const (
	CodeNotFound       Code = "NAMEPLATE_NOT_FOUND"
	CodeExpired        Code = "NAMEPLATE_EXPIRED"
	CodeAlreadyClaimed Code = "ALREADY_CLAIMED"
	CodePAKENotReady   Code = "PAKE_NOT_READY"
	CodePAKEInvalid    Code = "PAKE_INVALID"
)

var (
	ErrNotFound       = &ExchangeError{Code: CodeNotFound, Message: "nameplate not found or expired"}
	ErrExpired        = &ExchangeError{Code: CodeExpired, Message: "no receiver arrived within the time limit"}
	ErrAlreadyClaimed = &ExchangeError{Code: CodeAlreadyClaimed, Message: "nameplate has already been claimed"}
	ErrPAKENotReady   = &ExchangeError{Code: CodePAKENotReady, Message: "sender has not yet provided their PAKE message"}
	ErrPAKEInvalid    = &ExchangeError{Code: CodePAKEInvalid, Message: "PAKE body must be a non-empty base64 string"}
)

// ExchangeError is a rich error carrying a machine-readable Code and a human
// message, optionally wrapping an underlying cause.
type ExchangeError struct {
	Code    Code
	Message string
	cause   error
}

func (e *ExchangeError) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.cause)
	}
	return e.Message
}

func (e *ExchangeError) Unwrap() error { return e.cause }

// Is reports whether target is an *ExchangeError with the same Code.
// This allows errors.Is(err, ErrNotFound) to work even when err wraps ErrNotFound.
func (e *ExchangeError) Is(target error) bool {
	t, ok := target.(*ExchangeError)
	return ok && t.Code == e.Code
}

