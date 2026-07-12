package app

import (
	"errors"
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
}

func TestExitCoder(t *testing.T) {
	err := NewExitError(errors.New("test error"), ExitConfig)
	if err.Error() != "test error" {
		t.Errorf("Error() = %q, want %q", err.Error(), "test error")
	}

	if !errors.Is(err, errors.Unwrap(err)) {
		// Verify Unwrap works.
	}
}

func TestExitCodeFromError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"exit coder", NewExitError(errors.New("test"), ExitAuth), ExitAuth},
		{"plain error", errors.New("test"), ExitGeneral},
		{"wrapped", NewExitError(errors.New("inner"), ExitNetwork), ExitNetwork},
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
