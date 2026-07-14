package app

import (
	stderrors "errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"testing"
)

func TestProviderError_Error(t *testing.T) {
	tests := []struct {
		name string
		pe   *ProviderError
		want string
	}{
		{
			"status only",
			&ProviderError{StatusCode: 404, Detail: "not found"},
			"provider error 404: not found",
		},
		{
			"status with UPID",
			&ProviderError{StatusCode: 500, Detail: "fail", UPID: "UPID:pve1:123"},
			"provider error 500 (task UPID:pve1:123): fail",
		},
		{
			"ambiguous no status",
			&ProviderError{UPID: "UPID:pve1:456", Detail: "lost connection", Err: stderrors.New("EOF")},
			"ambiguous task outcome UPID:pve1:456: lost connection",
		},
		{
			"no status no UPID",
			&ProviderError{Detail: "network unreachable"},
			"provider error: network unreachable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.pe.Error()
			if got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestProviderError_Unwrap(t *testing.T) {
	inner := stderrors.New("inner error")
	pe := &ProviderError{StatusCode: 500, Detail: "outer", Err: inner}

	unwrap := stderrors.Unwrap(pe)
	if unwrap != inner {
		t.Errorf("Unwrap() = %v, want inner error", unwrap)
	}
}

func TestProviderError_IsTimeout(t *testing.T) {
	tests := []struct {
		name string
		pe   *ProviderError
		want bool
	}{
		{"504 status", &ProviderError{StatusCode: http.StatusGatewayTimeout}, true},
		{"timeout err", &ProviderError{Err: fmt.Errorf("i/o timeout")}, true},
		{"deadline exceeded err", &ProviderError{Err: os.ErrDeadlineExceeded}, true},
		{"not timeout", &ProviderError{StatusCode: 500, Detail: "error"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.pe.IsTimeout()
			if got != tt.want {
				t.Errorf("IsTimeout() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProviderError_IsCancellation(t *testing.T) {
	tests := []struct {
		name string
		pe   *ProviderError
		want bool
	}{
		{"canceled", &ProviderError{Err: fmt.Errorf("context canceled")}, true},
		{"not canceled", &ProviderError{Err: fmt.Errorf("some error")}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.pe.IsCancellation()
			if got != tt.want {
				t.Errorf("IsCancellation() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsAuthError(t *testing.T) {
	if !IsAuthError(&ProviderError{StatusCode: http.StatusUnauthorized}) {
		t.Error("expected IsAuthError for 401")
	}
	if IsAuthError(&ProviderError{StatusCode: http.StatusForbidden}) {
		t.Error("expected !IsAuthError for 403")
	}
}

func TestIsAuthorizationError(t *testing.T) {
	if !IsAuthorizationError(&ProviderError{StatusCode: http.StatusForbidden}) {
		t.Error("expected IsAuthorizationError for 403")
	}
}

func TestIsNotFoundError(t *testing.T) {
	if !IsNotFoundError(&ProviderError{StatusCode: http.StatusNotFound}) {
		t.Error("expected IsNotFoundError for 404")
	}
}

func TestIsConflictError(t *testing.T) {
	if !IsConflictError(&ProviderError{StatusCode: http.StatusConflict}) {
		t.Error("expected IsConflictError for 409")
	}
}

func TestIsRateLimitError(t *testing.T) {
	if !IsRateLimitError(&ProviderError{StatusCode: http.StatusTooManyRequests}) {
		t.Error("expected IsRateLimitError for 429")
	}
}

func TestIsValidationError(t *testing.T) {
	if !IsValidationError(&ProviderError{StatusCode: http.StatusBadRequest}) {
		t.Error("expected IsValidationError for 400")
	}
	if !IsValidationError(&ProviderError{StatusCode: http.StatusUnprocessableEntity}) {
		t.Error("expected IsValidationError for 422")
	}
}

func TestIsTimeoutError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"provider 504", &ProviderError{StatusCode: http.StatusGatewayTimeout}, true},
		{"provider timeout err", &ProviderError{Err: os.ErrDeadlineExceeded}, true},
		{"plain timeout", fmt.Errorf("i/o timeout"), true},
		{"plain deadline", fmt.Errorf("context deadline exceeded"), true},
		{"not timeout", fmt.Errorf("some error"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsTimeoutError(tt.err)
			if got != tt.want {
				t.Errorf("IsTimeoutError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsCancellationError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"canceled", fmt.Errorf("context canceled"), true},
		{"not canceled", fmt.Errorf("some error"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsCancellationError(tt.err)
			if got != tt.want {
				t.Errorf("IsCancellationError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsNetworkError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"connection refused", fmt.Errorf("dial tcp: connection refused"), true},
		{"no route", fmt.Errorf("no route to host"), true},
		{"dns", &net.DNSError{Err: "no such host", Name: "bad"}, true},
		{"not network", fmt.Errorf("something else"), false},
		{"nil", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsNetworkError(tt.err)
			if got != tt.want {
				t.Errorf("IsNetworkError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsAmbiguousOutcome(t *testing.T) {
	pe := &ProviderError{UPID: "UPID:test", Detail: "lost", Err: stderrors.New("EOF")}
	if !IsAmbiguousOutcome(pe) {
		t.Error("expected ambiguous outcome")
	}
	pe2 := &ProviderError{UPID: "UPID:test", Detail: "timeout", Err: os.ErrDeadlineExceeded}
	if IsAmbiguousOutcome(pe2) {
		t.Error("expected not ambiguous for timeout")
	}
}

func TestNewProviderError(t *testing.T) {
	pe := NewProviderError(404, "gone", nil)
	if pe.StatusCode != 404 {
		t.Errorf("status = %d", pe.StatusCode)
	}
	if pe.Detail != "gone" {
		t.Errorf("detail = %s", pe.Detail)
	}
}

func TestNewProviderErrorWithUPID(t *testing.T) {
	pe := NewProviderErrorWithUPID("UPID:x:1", "lost", stderrors.New("EOF"))
	if pe.UPID != "UPID:x:1" {
		t.Errorf("UPID = %s", pe.UPID)
	}
}

func TestHTTPStatusFromError(t *testing.T) {
	pe := &ProviderError{StatusCode: 404}
	if HTTPStatusFromError(pe) != 404 {
		t.Error("expected 404")
	}
	wrapped := fmt.Errorf("wrapped: %w", pe)
	if HTTPStatusFromError(wrapped) != 404 {
		t.Error("expected 404 from wrapped")
	}
	if HTTPStatusFromError(stderrors.New("plain")) != 0 {
		t.Error("expected 0 from plain")
	}
}

func TestUPIDFromError(t *testing.T) {
	pe := &ProviderError{UPID: "UPID:test:1"}
	if UPIDFromError(pe) != "UPID:test:1" {
		t.Error("expected UPID")
	}
	wrapped := fmt.Errorf("wrapped: %w", pe)
	if UPIDFromError(wrapped) != "UPID:test:1" {
		t.Error("expected UPID from wrapped")
	}
	if UPIDFromError(stderrors.New("plain")) != "" {
		t.Error("expected empty from plain")
	}
}
