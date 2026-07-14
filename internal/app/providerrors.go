// Package app provides application-level types including classified provider errors.
package app

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
)

// ProviderError is a classified error from a provider API call.
// It captures the HTTP status, optional UPID, and the underlying error
// so callers can distinguish between authentication, authorization,
// not-found, conflict, rate-limit, timeout, cancellation, and network failures.
type ProviderError struct {
	StatusCode int
	Detail     string
	UPID       string // set when a task was submitted but the outcome is ambiguous
	Err        error  // underlying error (transport, context, etc.)
}

func (e *ProviderError) Error() string {
	if e.UPID != "" && e.StatusCode == 0 {
		return fmt.Sprintf("ambiguous task outcome %s: %s", e.UPID, e.Detail)
	}
	if e.UPID != "" {
		return fmt.Sprintf("provider error %d (task %s): %s", e.StatusCode, e.UPID, e.Detail)
	}
	if e.StatusCode > 0 {
		return fmt.Sprintf("provider error %d: %s", e.StatusCode, e.Detail)
	}
	return fmt.Sprintf("provider error: %s", e.Detail)
}

// Unwrap returns the underlying error for errors.Is/As chain traversal.
func (e *ProviderError) Unwrap() error {
	return e.Err
}

// IsTimeout reports whether the error represents a timeout.
func (e *ProviderError) IsTimeout() bool {
	return e.StatusCode == http.StatusGatewayTimeout || classifyTimeout(e.Err)
}

// IsCancellation reports whether the error represents context cancellation.
func (e *ProviderError) IsCancellation() bool {
	return classifyCancellation(e.Err)
}

// IsAmbiguous reports whether the mutation was submitted but the final outcome is unknown.
func (e *ProviderError) IsAmbiguous() bool {
	return e.UPID != "" && e.Err != nil && !classifyTimeout(e.Err)
}

// --- Classification helpers using errors.As ---

// HTTPStatusFromError extracts the HTTP status code from a ProviderError chain.
// Returns 0 if no ProviderError is found.
func HTTPStatusFromError(err error) int {
	var pe *ProviderError
	if errors.As(err, &pe) {
		return pe.StatusCode
	}
	return 0
}

// UPIDFromError extracts the UPID from a ProviderError chain.
// Returns "" if no ProviderError is found or no UPID is set.
func UPIDFromError(err error) string {
	var pe *ProviderError
	if errors.As(err, &pe) {
		return pe.UPID
	}
	return ""
}

// IsAuthError reports whether the error chain contains a 401 status.
func IsAuthError(err error) bool {
	var pe *ProviderError
	return errors.As(err, &pe) && pe.StatusCode == http.StatusUnauthorized
}

// IsAuthorizationError reports whether the error chain contains a 403 status.
func IsAuthorizationError(err error) bool {
	var pe *ProviderError
	return errors.As(err, &pe) && pe.StatusCode == http.StatusForbidden
}

// IsNotFoundError reports whether the error chain contains a 404 status.
func IsNotFoundError(err error) bool {
	var pe *ProviderError
	return errors.As(err, &pe) && pe.StatusCode == http.StatusNotFound
}

// IsConflictError reports whether the error chain contains a 409 status.
func IsConflictError(err error) bool {
	var pe *ProviderError
	return errors.As(err, &pe) && pe.StatusCode == http.StatusConflict
}

// IsRateLimitError reports whether the error chain contains a 429 status.
func IsRateLimitError(err error) bool {
	var pe *ProviderError
	return errors.As(err, &pe) && pe.StatusCode == http.StatusTooManyRequests
}

// IsValidationError reports whether the error chain contains a 400 or 422 status.
func IsValidationError(err error) bool {
	var pe *ProviderError
	return errors.As(err, &pe) &&
		(pe.StatusCode == http.StatusBadRequest || pe.StatusCode == http.StatusUnprocessableEntity)
}

// IsTimeoutError reports whether the error chain represents any kind of timeout.
func IsTimeoutError(err error) bool {
	var pe *ProviderError
	if errors.As(err, &pe) && pe.IsTimeout() {
		return true
	}
	return classifyTimeout(err)
}

// IsCancellationError reports whether the error chain represents context cancellation.
func IsCancellationError(err error) bool {
	var pe *ProviderError
	if errors.As(err, &pe) && pe.IsCancellation() {
		return true
	}
	return classifyCancellation(err)
}

// IsNetworkError reports whether the error chain represents a network-level failure.
func IsNetworkError(err error) bool {
	var pe *ProviderError
	if errors.As(err, &pe) {
		// Network errors typically have status code 0 (no HTTP response received).
		if pe.StatusCode == 0 && pe.Err != nil {
			return true
		}
	}
	return classifyNetwork(err)
}

// IsAmbiguousOutcome reports whether the mutation was submitted but the outcome is unknown.
func IsAmbiguousOutcome(err error) bool {
	var pe *ProviderError
	return errors.As(err, &pe) && pe.IsAmbiguous()
}

// TaskUPIDFromError extracts the task UPID from a ProviderError chain.
// Deprecated: Use UPIDFromError.
func TaskUPIDFromError(err error) string {
	return UPIDFromError(err)
}

// NewProviderError creates a ProviderError with the given status code and detail.
func NewProviderError(statusCode int, detail string, underlying error) *ProviderError {
	return &ProviderError{
		StatusCode: statusCode,
		Detail:     detail,
		Err:        underlying,
	}
}

// NewProviderErrorWithUPID creates a ProviderError for a task whose outcome is unknown.
func NewProviderErrorWithUPID(upid, detail string, underlying error) *ProviderError {
	return &ProviderError{
		UPID:   upid,
		Detail: detail,
		Err:    underlying,
	}
}

// --- Private classification helpers ---

// classifyTimeout checks whether err represents a timeout, including
// context.DeadlineExceeded and os.ErrDeadlineExceeded.
func classifyTimeout(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}
	// Check for context.DeadlineExceeded by message — avoids importing context.
	msg := err.Error()
	return strings.Contains(msg, "deadline exceeded") ||
		strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "context deadline exceeded")
}

// classifyCancellation checks whether err represents cancellation.
func classifyCancellation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "canceled") ||
		strings.Contains(msg, "cancelled") ||
		strings.Contains(msg, "context canceled")
}

// classifyNetwork checks whether err represents a network-level failure.
func classifyNetwork(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "no route to host") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "network is unreachable") ||
		strings.Contains(msg, "EOF") ||
		errors.Is(err, os.ErrDeadlineExceeded)
}
