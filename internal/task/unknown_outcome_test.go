package task

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// --- UnknownOutcome type tests ---

func TestUnknownOutcomeErrorWithoutEndpoint(t *testing.T) {
	uo := &UnknownOutcome{
		UPID:  "UPID:pve1/100/0",
		Node:  "pve1",
		Cause: context.DeadlineExceeded,
	}
	msg := uo.Error()
	if msg == "" {
		t.Fatal("Error() returned empty string")
	}
	if !errors.Is(uo, context.DeadlineExceeded) {
		t.Error("UnknownOutcome should wrap context.DeadlineExceeded")
	}
	// Verify UPID and node appear in the message.
	assertContains(t, msg, "UPID:pve1/100/0")
	assertContains(t, msg, "pve1")
	assertNotContains(t, msg, "endpoint")
}

func TestUnknownOutcomeErrorWithEndpoint(t *testing.T) {
	uo := &UnknownOutcome{
		UPID:     "UPID:pve1/100/0",
		Node:     "pve1",
		Endpoint: "https://pve.example.com:8006",
		Cause:    context.DeadlineExceeded,
	}
	msg := uo.Error()
	assertContains(t, msg, "UPID:pve1/100/0")
	assertContains(t, msg, "pve1")
	assertContains(t, msg, "https://pve.example.com:8006")
	assertContains(t, msg, "endpoint")
}

func TestUnknownOutcomeUnwrap(t *testing.T) {
	inner := fmt.Errorf("inner cause")
	uo := &UnknownOutcome{
		UPID:  "UPID:pve1/100/0",
		Node:  "pve1",
		Cause: inner,
	}
	if !errors.Is(uo, inner) {
		t.Error("Unwrap should allow errors.Is to reach inner cause")
	}
}

func TestUnknownOutcomeWrapsSentinel(t *testing.T) {
	uo := newUnknownOutcome("UPID:pve1/100/0", "pve1", "")
	if !errors.Is(uo, context.DeadlineExceeded) {
		t.Error("newUnknownOutcome should wrap context.DeadlineExceeded")
	}
}

func TestUnknownOutcomeFromErr(t *testing.T) {
	cause := context.DeadlineExceeded
	uo := newUnknownOutcomeFromErr("UPID:pve1/100/0", "pve1", "https://pve:8006", cause)
	if !errors.Is(uo, cause) {
		t.Error("newUnknownOutcomeFromErr should wrap the provided cause")
	}
	if uo.Endpoint != "https://pve:8006" {
		t.Errorf("Endpoint = %q, want https://pve:8006", uo.Endpoint)
	}
}

// --- IsUnknownOutcome tests ---

func TestIsUnknownOutcomeTrue(t *testing.T) {
	err := newUnknownOutcome("UPID:pve1/100/0", "pve1", "")
	if !IsUnknownOutcome(err) {
		t.Error("IsUnknownOutcome should return true for *UnknownOutcome")
	}
}

func TestIsUnknownOutcomeWrapped(t *testing.T) {
	err := fmt.Errorf("outer: %w", newUnknownOutcome("UPID:pve1/100/0", "pve1", ""))
	if !IsUnknownOutcome(err) {
		t.Error("IsUnknownOutcome should return true for wrapped *UnknownOutcome")
	}
}

func TestIsUnknownOutcomeFalse(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"nil", nil},
		{"plain error", errors.New("some error")},
		{"deadline exceeded", context.DeadlineExceeded},
		{"canceled", context.Canceled},
		{"wrapped non-unknown", fmt.Errorf("wrapped: %w", context.DeadlineExceeded)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if IsUnknownOutcome(tt.err) {
				t.Errorf("IsUnknownOutcome(%v) should be false", tt.err)
			}
		})
	}
}

// --- AsUnknownOutcome tests ---

func TestAsUnknownOutcomePresent(t *testing.T) {
	original := &UnknownOutcome{
		UPID:  "UPID:pve1/100/0",
		Node:  "pve1",
		Cause: context.DeadlineExceeded,
	}
	extracted := AsUnknownOutcome(original)
	if extracted == nil {
		t.Fatal("AsUnknownOutcome should return non-nil for *UnknownOutcome")
	}
	if extracted.UPID != original.UPID {
		t.Errorf("UPID = %q, want %q", extracted.UPID, original.UPID)
	}
	if extracted.Node != original.Node {
		t.Errorf("Node = %q, want %q", extracted.Node, original.Node)
	}
}

func TestAsUnknownOutcomeWrapped(t *testing.T) {
	original := &UnknownOutcome{
		UPID:     "UPID:pve1/100/0",
		Node:     "pve1",
		Endpoint: "https://pve:8006",
		Cause:    context.DeadlineExceeded,
	}
	wrapped := fmt.Errorf("context: %w", original)
	extracted := AsUnknownOutcome(wrapped)
	if extracted == nil {
		t.Fatal("AsUnknownOutcome should unwrap to find *UnknownOutcome")
	}
	if extracted.Endpoint != "https://pve:8006" {
		t.Errorf("Endpoint = %q, want https://pve:8006", extracted.Endpoint)
	}
}

func TestAsUnknownOutcomeNil(t *testing.T) {
	if AsUnknownOutcome(nil) != nil {
		t.Error("AsUnknownOutcome(nil) should return nil")
	}
}

func TestAsUnknownOutcomeAbsent(t *testing.T) {
	err := errors.New("not an unknown outcome")
	if AsUnknownOutcome(err) != nil {
		t.Error("AsUnknownOutcome should return nil for non-UnknownOutcome error")
	}
}

// --- errors.As round-trip tests ---

func TestErrorsAsRoundTrip(t *testing.T) {
	original := newUnknownOutcome("UPID:pve1/100/0", "pve1", "")
	wrapped := fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", original))

	var target *UnknownOutcome
	if !errors.As(wrapped, &target) {
		t.Fatal("errors.As should find *UnknownOutcome through two levels of wrapping")
	}
	if target.UPID != "UPID:pve1/100/0" {
		t.Errorf("UPID = %q, want UPID:pve1/100/0", target.UPID)
	}
	if target.Node != "pve1" {
		t.Errorf("Node = %q, want pve1", target.Node)
	}
}

// --- Integration with app-level timeout detection ---

func TestUnknownOutcomeIsDetectedAsTimeout(t *testing.T) {
	// UnknownOutcome wraps context.DeadlineExceeded, so any code that
	// uses errors.Is(err, context.DeadlineExceeded) or checks the
	// "deadline exceeded" message pattern will correctly classify it.
	uo := newUnknownOutcome("UPID:pve1/100/0", "pve1", "")
	if !errors.Is(uo, context.DeadlineExceeded) {
		t.Error("UnknownOutcome should be detected as deadline exceeded via errors.Is")
	}
}

// --- Poller integration tests ---

func TestPollerTimeoutReturnsUnknownOutcome(t *testing.T) {
	mock := &mockTaskClient{
		statuses: []*TaskStatus{
			{UPID: "UPID:pve1/100/0", State: StateRunning},
			{UPID: "UPID:pve1/100/0", State: StateRunning},
			{UPID: "UPID:pve1/100/0", State: StateRunning},
		},
	}
	poller := NewPoller(mock,
		WithPollInterval(10*time.Millisecond),
		WithMaxWait(25*time.Millisecond),
	)
	result := poller.Wait(context.Background(), "pve1", "UPID:pve1/100/0")
	if result.Error == nil {
		t.Fatal("expected error from timeout")
	}
	if !IsUnknownOutcome(result.Error) {
		t.Errorf("expected *UnknownOutcome, got %T: %v", result.Error, result.Error)
	}
	uo := AsUnknownOutcome(result.Error)
	if uo == nil {
		t.Fatal("AsUnknownOutcome returned nil")
	}
	if uo.UPID != "UPID:pve1/100/0" {
		t.Errorf("UPID = %q, want UPID:pve1/100/0", uo.UPID)
	}
	if uo.Node != "pve1" {
		t.Errorf("Node = %q, want pve1", uo.Node)
	}
	if !errors.Is(uo, context.DeadlineExceeded) {
		t.Error("UnknownOutcome should wrap context.DeadlineExceeded")
	}
}

func TestPollerTimeoutWithEndpoint(t *testing.T) {
	mock := &mockTaskClient{
		statuses: []*TaskStatus{
			{UPID: "UPID:pve1/100/0", State: StateRunning},
		},
	}
	poller := NewPoller(mock,
		WithPollInterval(10*time.Millisecond),
		WithMaxWait(25*time.Millisecond),
		WithEndpoint("https://pve.example.com:8006"),
	)
	result := poller.Wait(context.Background(), "pve1", "UPID:pve1/100/0")
	if result.Error == nil {
		t.Fatal("expected error from timeout")
	}
	uo := AsUnknownOutcome(result.Error)
	if uo == nil {
		t.Fatal("expected *UnknownOutcome")
	}
	if uo.Endpoint != "https://pve.example.com:8006" {
		t.Errorf("Endpoint = %q, want https://pve.example.com:8006", uo.Endpoint)
	}
}

func TestPollerContextDeadlineReturnsUnknownOutcome(t *testing.T) {
	mock := &mockTaskClient{
		statuses: []*TaskStatus{
			{UPID: "UPID:pve1/100/0", State: StateRunning},
			{UPID: "UPID:pve1/100/0", State: StateRunning},
			{UPID: "UPID:pve1/100/0", State: StateRunning},
		},
	}
	poller := NewPoller(mock,
		WithPollInterval(50*time.Millisecond),
		WithMaxWait(5*time.Second),
	)
	// Set a context deadline that will be exceeded.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	result := poller.Wait(ctx, "pve1", "UPID:pve1/100/0")
	if result.Error == nil {
		t.Fatal("expected error from context deadline")
	}
	if !IsUnknownOutcome(result.Error) {
		t.Errorf("expected *UnknownOutcome for context deadline exceeded, got %T: %v", result.Error, result.Error)
	}
	if result.UPID != "UPID:pve1/100/0" {
		t.Errorf("UPID = %q, want UPID:pve1/100/0", result.UPID)
	}
}

func TestPollerContextCancellationStillReturnsPlainError(t *testing.T) {
	mock := &mockTaskClient{
		statuses: []*TaskStatus{
			{UPID: "UPID:pve1/100/0", State: StateRunning},
			{UPID: "UPID:pve1/100/0", State: StateRunning},
			{UPID: "UPID:pve1/100/0", State: StateRunning},
		},
	}
	poller := NewPoller(mock,
		WithPollInterval(50*time.Millisecond),
		WithMaxWait(5*time.Second),
	)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	result := poller.Wait(ctx, "pve1", "UPID:pve1/100/0")
	if result.Error == nil {
		t.Fatal("expected error from cancellation")
	}
	// Explicit cancellation must NOT be an UnknownOutcome.
	if IsUnknownOutcome(result.Error) {
		t.Error("explicit cancellation should not produce *UnknownOutcome")
	}
	if result.UPID != "UPID:pve1/100/0" {
		t.Errorf("UPID = %q, want UPID:pve1/100/0", result.UPID)
	}
}

func TestPollerDeadlineViaQueryErrorRetry(t *testing.T) {
	// Simulate a client that always returns errors, so the poller retries
	// until the context deadline is exceeded during the backoff select.
	mock := &alwaysFailClient{}
	poller := NewPoller(mock,
		WithPollInterval(10*time.Millisecond),
		WithMaxWait(5*time.Second),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	result := poller.Wait(ctx, "pve1", "UPID:pve1/100/0")
	if result.Error == nil {
		t.Fatal("expected error from context deadline during retry")
	}
	if !IsUnknownOutcome(result.Error) {
		t.Errorf("expected *UnknownOutcome for deadline during retry, got %T: %v", result.Error, result.Error)
	}
}

// --- helpers ---

type alwaysFailClient struct{}

func (c *alwaysFailClient) GetTask(_ context.Context, _, _ string) (*TaskStatus, error) {
	return nil, errors.New("persistent failure")
}

func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("string %q does not contain %q", s, substr)
	}
}

func assertNotContains(t *testing.T, s, substr string) {
	t.Helper()
	if strings.Contains(s, substr) {
		t.Errorf("string %q should not contain %q", s, substr)
	}
}
