package temuops

import "fmt"

var (
	ErrInvalid      = &Error{Kind: "invalid"}
	ErrNotFound     = &Error{Kind: "not_found"}
	ErrConflict     = &Error{Kind: "conflict"}
	ErrCredentials  = &Error{Kind: "credentials"}
	ErrAPIGap       = &Error{Kind: "api_gap"}
	ErrLiveDisabled = &Error{Kind: "live_disabled"}
)

// Error is a user-facing failure that still unwraps to a sentinel kind.
type Error struct {
	Kind string
	Msg  string
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Msg != "" {
		return e.Msg
	}
	return e.Kind
}

func (e *Error) Is(target error) bool {
	other, ok := target.(*Error)
	return ok && other != nil && e != nil && e.Kind == other.Kind
}

func invalid(msg string) error { return &Error{Kind: ErrInvalid.Kind, Msg: msg} }

func notFound(msg string) error { return &Error{Kind: ErrNotFound.Kind, Msg: msg} }

func conflict(msg string) error { return &Error{Kind: ErrConflict.Kind, Msg: msg} }

func failed(msg string) error { return fmt.Errorf("%s", msg) }
