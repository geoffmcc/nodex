package app

import (
	"errors"
	"net/http"
)

// Exit codes matching the product specification.
// Range 0-99: application-defined; 100+: standard signals.
const (
	ExitSuccess          = 0
	ExitGeneral          = 1
	ExitUsage            = 2
	ExitConfig           = 3
	ExitCredential       = 4
	ExitAuth             = 5
	ExitAuthorization    = 6
	ExitNetwork          = 7
	ExitTLS              = 8
	ExitIncompatibility  = 9
	ExitUnsupportedCap   = 10
	ExitPartialFailure   = 11
	ExitProvider         = 12
	ExitNotFound         = 13
	ExitTimeout          = 14
	ExitCancellation     = 15
	ExitTaskFailure      = 16
	ExitValidationError  = 17
	ExitAmbiguousOutcome = 18
	ExitRateLimit        = 19
	ExitOutputError      = 20
	ExitConflict         = 21
	ExitInterrupted      = 130
	ExitSigterm          = 143
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
	ErrUnsupportedCap  = errors.New("unsupported capability")
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

// EmittedError marks an error whose user-facing details were already written
// to the output stream (for example inside an OperationResult envelope), so a
// command frontend must not emit a second error document. The wrapped error's
// chain is preserved for exit-code classification and errors.Is/As.
type EmittedError struct {
	Err error
}

func (e *EmittedError) Error() string {
	return e.Err.Error()
}

func (e *EmittedError) Unwrap() error {
	return e.Err
}

// MarkEmitted wraps err to record that its details were already emitted.
// It is a no-op for nil errors and for errors already marked as emitted.
func MarkEmitted(err error) error {
	if err == nil || IsEmitted(err) {
		return err
	}
	return &EmittedError{Err: err}
}

// IsEmitted reports whether err, or any error wrapping it, was marked as
// already emitted via MarkEmitted.
func IsEmitted(err error) bool {
	var ee *EmittedError
	return errors.As(err, &ee)
}

// ExitCodeFromError extracts the exit code from an error chain.
// It checks for typed ProviderError first, then ExitCoder, then
// classifies by string pattern as a final fallback.
// Returns ExitGeneral (1) when no classification is possible.
func ExitCodeFromError(err error) int {
	if err == nil {
		return ExitSuccess
	}

	// 1. Check for provider-error classification (HTTP status, timeout, network, etc.).
	if code := classifyProviderError(err); code != ExitGeneral {
		return code
	}

	// 2. Check for explicit ExitCoder wrapping.
	var ec *ExitCoder
	if errors.As(err, &ec) {
		return ec.ExitCode
	}

	// 3. Fallback string-pattern classification.
	if code := classifyByPattern(err); code != ExitGeneral {
		return code
	}

	return ExitGeneral
}

// classifyProviderError maps a ProviderError to the appropriate exit code.
// Returns ExitGeneral when the error is not a ProviderError.
func classifyProviderError(err error) int {
	var pe *ProviderError
	if !errors.As(err, &pe) {
		return ExitGeneral
	}

	// UPID-bearing errors (ambiguous outcome).
	if pe.UPID != "" && pe.StatusCode == 0 {
		if pe.IsTimeout() {
			return ExitTimeout
		}
		return ExitAmbiguousOutcome
	}

	switch pe.StatusCode {
	case http.StatusUnauthorized:
		return ExitAuth
	case http.StatusForbidden:
		return ExitAuthorization
	case http.StatusNotFound:
		return ExitNotFound
	case http.StatusConflict:
		return ExitConflict
	case http.StatusTooManyRequests:
		return ExitRateLimit
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return ExitValidationError
	case http.StatusGatewayTimeout:
		return ExitTimeout
	case 0:
		// No HTTP status: classify by underlying error.
		if pe.IsTimeout() {
			return ExitTimeout
		}
		if pe.IsCancellation() {
			return ExitCancellation
		}
		if IsNetworkError(pe.Err) {
			return ExitNetwork
		}
		return ExitProvider
	default:
		// 500, 502, 503, etc.
		return ExitProvider
	}
}

// classifyByPattern uses string matching against the error and any
// wrapped errors to determine an exit code. This is a last-resort
// classification for errors that haven't been wrapped in ProviderError
// or ExitCoder.
func classifyByPattern(err error) int {
	msg := err.Error()

	// Check for timeout indicators.
	if IsTimeoutError(err) {
		return ExitTimeout
	}

	// Check for cancellation indicators.
	if IsCancellationError(err) {
		return ExitCancellation
	}

	// Check for network failure indicators.
	if IsNetworkError(err) {
		return ExitNetwork
	}

	// Check for auth indicators.
	if IsAuthError(err) {
		return ExitAuth
	}

	// Check for not-found indicators.
	if IsNotFoundError(err) {
		return ExitNotFound
	}

	// Re-check timeout/cancellation via string matching on the top-level message.
	if msg == "context deadline exceeded" || msg == "context canceled" {
		if msg == "context canceled" {
			return ExitCancellation
		}
		return ExitTimeout
	}

	return ExitGeneral
}
