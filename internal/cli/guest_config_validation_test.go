package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/app"
)

func TestValidateGuestConfigValueTypes(t *testing.T) {
	tests := []struct {
		name   string
		kind   guestConfigKind
		params map[string]string
		want   string
	}{
		{
			name: "VM numeric values",
			kind: guestConfigVM,
			params: map[string]string{
				"cores": "4", "sockets": "2", "memory": "4096", "balloon": "0", "cpuunits": "100",
			},
		},
		{
			name: "container numeric values",
			kind: guestConfigCT,
			params: map[string]string{
				"cores": "2", "memory": "2048", "swap": "0", "cpuunits": "100",
			},
		},
		{
			name: "unknown version-specific keys are left to the provider",
			kind: guestConfigVM,
			params: map[string]string{
				"future-option": "custom=value",
				"description":   "contains=equals",
			},
		},
		{
			name:   "invalid VM memory type",
			kind:   guestConfigVM,
			params: map[string]string{"memory": "4GiB"},
			want:   `invalid VM configuration value for "memory": expected an integer`,
		},
		{
			name:   "invalid container cores type",
			kind:   guestConfigCT,
			params: map[string]string{"cores": "two"},
			want:   `invalid container configuration value for "cores": expected an integer`,
		},
		{
			name: "VM-only key is not type-checked as a container key",
			kind: guestConfigCT,
			params: map[string]string{
				"sockets": "provider-specific-value",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateGuestConfigValueTypes(tt.kind, tt.params)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("validateGuestConfigValueTypes() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("validateGuestConfigValueTypes() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestValidateGuestConfigValueTypesRejectsUnknownKind(t *testing.T) {
	if err := validateGuestConfigValueTypes("other", map[string]string{"cores": "2"}); err == nil {
		t.Fatal("expected unsupported guest kind error")
	}
}

func TestGuestConfigUpdatesRejectMalformedIntegerBeforeProviderCall(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "VM",
			args: []string{"--yes", "vm", "update", "e2e-node/100", "memory=4GiB"},
			want: `invalid VM configuration value for "memory": expected an integer`,
		},
		{
			name: "container",
			args: []string{"--yes", "container", "update", "e2e-node/200", "cores=two"},
			want: `invalid container configuration value for "cores": expected an integer`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateConfigAndHome(t)
			setupE2EConfig(t)

			var stdout, stderr bytes.Buffer
			err := Run(context.Background(), tt.args, &stdout, &stderr)
			if app.ExitCodeFromError(err) != app.ExitUsage {
				t.Fatalf("Run(%v) error = %v, want usage error", tt.args, err)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Run(%v) error = %v, want substring %q", tt.args, err, tt.want)
			}
			if stdout.Len() != 0 {
				t.Fatalf("invalid update wrote output, expected provider call to be skipped: %s", stdout.String())
			}
		})
	}
}
