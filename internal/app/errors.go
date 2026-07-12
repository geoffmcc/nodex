package app

import "errors"

// Exit codes matching the product specification.
const (
	ExitSuccess         = 0
	ExitGeneral         = 1
	ExitUsage           = 2
	ExitConfig          = 3
	ExitCredential      = 4
	ExitAuth            = 5
	ExitAuthorization   = 6
	ExitNetwork         = 7
	ExitTLS             = 8
	ExitIncompatibility = 9
	ExitUnsupportedCap  = 10
	ExitPartialFailure  = 11
	ExitInterrupted     = 130
	ExitSigterm         = 143
)

// Typed errors for structured error handling.
var (
	ErrConfigRead      = errors.New("config read failed")
	ErrConfigWrite     = errors.New("config write failed")
	ErrConfigInvalid   = errors.New("config invalid")
	ErrProfileNotFound = errors.New("profile not found")
	ErrProfileExists   = errors.New("profile already exists")
	ErrProfileInvalid  = errors.New("profile invalid")
	ErrNoProfile       = errors.New("no profile configured")
	ErrCredential      = errors.New("credential unavailable")
	ErrAuth            = errors.New("authentication failed")
	ErrTLS             = errors.New("TLS error")
	ErrNetwork         = errors.New("network error")
	ErrProvider        = errors.New("provider error")
	ErrRedaction       = errors.New("redaction error")
)

// ExitCoder wraps an error with an exit code.
type ExitCoder struct {
	Err      error
	ExitCode int
}

func (e *ExitCoder) Error() string {
	return e.Err.Error()
}

func (e *ExitCoder) Unwrap() error {
	return e.Err
}

// NewExitError wraps an error with an exit code.
func NewExitError(err error, code int) *ExitCoder {
	return &ExitCoder{Err: err, ExitCode: code}
}

// ExitCodeFromError extracts the exit code from an error chain.
// Returns ExitGeneral (1) if no ExitCoder is found.
func ExitCodeFromError(err error) int {
	var ec *ExitCoder
	if errors.As(err, &ec) {
		return ec.ExitCode
	}
	return ExitGeneral
}
