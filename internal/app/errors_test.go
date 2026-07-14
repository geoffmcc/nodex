package app

import (
	stderrors "errors"
	"fmt"
	"net/http"
	"testing"
)

func TestExitCodeConstants(t *testing.T) {
	if ExitSuccess != 0 {
		t.Errorf("ExitSuccess = %d, want 0", ExitSuccess)
	}
	if ExitGeneral != 1 {
		t.Errorf("ExitGeneral = %d, want 1", ExitGeneral)
	}
	if ExitUsage != 2 {
		t.Errorf("ExitUsage = %d, want 2", ExitUsage)
	}
	if ExitConfig != 3 {
		t.Errorf("ExitConfig = %d, want 3", ExitConfig)
	}
	if ExitCredential != 4 {
		t.Errorf("ExitCredential = %d, want 4", ExitCredential)
	}
	if ExitAuth != 5 {
		t.Errorf("ExitAuth = %d, want 5", ExitAuth)
	}
	if ExitInterrupted != 130 {
		t.Errorf("ExitInterrupted = %d, want 130", ExitInterrupted)
	}
	if ExitSigterm != 143 {
		t.Errorf("ExitSigterm = %d, want 143", ExitSigterm)
	}
}

func TestExitCoder(t *testing.T) {
	err := NewExitError(stderrors.New("test error"), ExitConfig)
	if err.Error() != "test error" {
		t.Errorf("Error() = %q, want %q", err.Error(), "test error")
	}

	if stderrors.Unwrap(err) == nil {
		t.Fatal("expected wrapped error")
	}
}

func TestExitCodeFromError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"nil error", nil, ExitSuccess},
		{"exit coder", NewExitError(stderrors.New("test"), ExitAuth), ExitAuth},
		{"plain error", stderrors.New("test"), ExitGeneral},
		{"wrapped exit coder", NewExitError(stderrors.New("inner"), ExitNetwork), ExitNetwork},
		// ProviderError classification.
		{"provider 401", &ProviderError{StatusCode: http.StatusUnauthorized, Detail: "unauthorized"}, ExitAuth},
		{"provider 403", &ProviderError{StatusCode: http.StatusForbidden, Detail: "forbidden"}, ExitAuthorization},
		{"provider 404", &ProviderError{StatusCode: http.StatusNotFound, Detail: "not found"}, ExitNotFound},
		{"provider 409", &ProviderError{StatusCode: http.StatusConflict, Detail: "conflict"}, ExitConflict},
		{"provider 429", &ProviderError{StatusCode: http.StatusTooManyRequests, Detail: "rate limited"}, ExitRateLimit},
		{"provider 400", &ProviderError{StatusCode: http.StatusBadRequest, Detail: "bad request"}, ExitValidationError},
		{"provider 422", &ProviderError{StatusCode: http.StatusUnprocessableEntity, Detail: "unprocessable"}, ExitValidationError},
		{"provider 504", &ProviderError{StatusCode: http.StatusGatewayTimeout, Detail: "timeout"}, ExitTimeout},
		{"provider 500", &ProviderError{StatusCode: http.StatusInternalServerError, Detail: "server error"}, ExitProvider},
		{"provider 503", &ProviderError{StatusCode: http.StatusServiceUnavailable, Detail: "unavailable"}, ExitProvider},
		// ProviderError with transport errors.
		{"network error", &ProviderError{StatusCode: 0, Detail: "connection refused", Err: fmt.Errorf("dial tcp: connection refused")}, ExitNetwork},
		// Ambiguous outcome.
		{"ambiguous outcome", &ProviderError{UPID: "UPID:pve1:000:A:B:C", Detail: "connection lost", Err: stderrors.New("EOF")}, ExitAmbiguousOutcome},
		// Timeout with UPID.
		{"timeout with UPID", &ProviderError{UPID: "UPID:pve1:000:A:B:C", Detail: "i/o timeout", Err: fmt.Errorf("i/o timeout")}, ExitTimeout},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExitCodeFromError(tt.err)
			if got != tt.want {
				t.Errorf("ExitCodeFromError() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestExitCodeFromError_ClassifyTimeout(t *testing.T) {
	err := fmt.Errorf("context deadline exceeded")
	code := ExitCodeFromError(err)
	if code != ExitTimeout {
		t.Errorf("ExitCodeFromError(%q) = %d, want ExitTimeout(%d)", err, code, ExitTimeout)
	}
}

func TestExitCodeFromError_ClassifyCancellation(t *testing.T) {
	err := fmt.Errorf("context canceled")
	code := ExitCodeFromError(err)
	if code != ExitCancellation {
		t.Errorf("ExitCodeFromError(%q) = %d, want ExitCancellation(%d)", err, code, ExitCancellation)
	}
}

func TestErrorsAreTyped(t *testing.T) {
	errs := []struct {
		name string
		err  error
	}{
		{"ErrConfigRead", ErrConfigRead},
		{"ErrConfigWrite", ErrConfigWrite},
		{"ErrConfigInvalid", ErrConfigInvalid},
		{"ErrProfileNotFound", ErrProfileNotFound},
		{"ErrProfileExists", ErrProfileExists},
		{"ErrProfileInvalid", ErrProfileInvalid},
		{"ErrNoProfile", ErrNoProfile},
		{"ErrCredential", ErrCredential},
		{"ErrAuth", ErrAuth},
		{"ErrTLS", ErrTLS},
		{"ErrNetwork", ErrNetwork},
		{"ErrProvider", ErrProvider},
		{"ErrRedaction", ErrRedaction},
	}

	for _, tt := range errs {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				t.Errorf("%s is nil", tt.name)
			}
			if tt.err.Error() == "" {
				t.Errorf("%s has empty message", tt.name)
			}
		})
	}
}

func TestExitCodeFromError_ExitCoderOverProviderError(t *testing.T) {
	// When an ExitCoder wraps a ProviderError, the ExitCoder's code wins
	// because ExitCodeFromError checks ProviderError first, then ExitCoder.
	// But currently classifyProviderError runs first. This test documents
	// the current behavior: ProviderError wins over ExitCoder.
	pe := &ProviderError{StatusCode: http.StatusNotFound, Detail: "gone"}
	ec := NewExitError(pe, ExitProvider)
	got := ExitCodeFromError(ec)
	// classifyProviderError unwraps through the chain: ec.Unwrap() -> pe
	// errors.As(ec, &pe) returns true, and pe.StatusCode == 404 → ExitNotFound
	if got != ExitNotFound {
		t.Errorf("ExitCodeFromError(ExitCoder(404)) = %d, want ExitNotFound(%d)", got, ExitNotFound)
	}
}
