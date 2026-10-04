package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestGuestConfigUpdateConfirmationMessageRedactsAndSorts(t *testing.T) {
	message := guestConfigUpdateConfirmationMessage("Operation on VM pve1/100.", map[string]string{
		"memory":        "4096",
		"cipassword":    "fixture",
		"future-option": "unrecognized-fixture",
		"name":          "web",
		"sshkeys":       "ssh-public-key-fixture",
	})

	for _, want := range []string{
		"Requested changes (sensitive or unrecognized values are redacted):",
		"cipassword=[REDACTED]",
		"future-option=[REDACTED]",
		"memory=4096",
		"name=web",
		"sshkeys=[REDACTED]",
	} {
		if !strings.Contains(message, want) {
			t.Errorf("confirmation message missing %q:\n%s", want, message)
		}
	}
	for _, value := range []string{"fixture", "unrecognized-fixture", "ssh-public-key-fixture"} {
		if strings.Contains(message, value) {
			t.Errorf("confirmation message exposed a redacted value %q:\n%s", value, message)
		}
	}
	if strings.Index(message, "cipassword=") > strings.Index(message, "future-option=") || strings.Index(message, "future-option=") > strings.Index(message, "memory=") {
		t.Fatalf("confirmation fields are not deterministically sorted:\n%s", message)
	}
}

func TestGuestConfigPreviewEscapesControlsAndTruncatesValues(t *testing.T) {
	longValue := strings.Repeat("é", maxGuestConfigPreviewValueRunes+10)
	message := guestConfigUpdateConfirmationMessage("confirm", map[string]string{
		"name":        "web\nforged preview",
		"future\nkey": "ignored-secret-value",
		"tags":        longValue,
	})

	if strings.Contains(message, "web\nforged preview") || !strings.Contains(message, `name=web\nforged preview`) {
		t.Fatalf("control character was not escaped in preview:\n%s", message)
	}
	if !strings.Contains(message, `future\nkey=[REDACTED]`) {
		t.Fatalf("unrecognized key was not safely escaped and redacted:\n%s", message)
	}
	if !strings.Contains(message, strings.Repeat("é", maxGuestConfigPreviewValueRunes)+"...") {
		t.Fatalf("long safe value was not truncated:\n%s", message)
	}
}

func TestGuestConfigPreviewRedactsSecretAssignmentsInsideAllowedValue(t *testing.T) {
	message := guestConfigUpdateConfirmationMessage("confirm", map[string]string{
		"name": "node password=fixture",
	})
	if strings.Contains(message, "fixture") {
		t.Fatalf("free-text secret in an allowlisted value was exposed:\n%s", message)
	}
	if !strings.Contains(message, "name=node password=[REDACTED]") {
		t.Fatalf("expected free-text redaction in preview:\n%s", message)
	}
}

func TestVMAndContainerUpdatePromptsIncludeRedactedPreviews(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		shown  []string
		hidden []string
	}{
		{
			name:   "VM",
			args:   []string{"vm", "update", "e2e-node/100", "memory=4096", "cipassword=fixture", "future-option=unrecognized-fixture"},
			shown:  []string{"memory=4096", "cipassword=[REDACTED]", "future-option=[REDACTED]"},
			hidden: []string{"fixture", "unrecognized-fixture"},
		},
		{
			name:   "container",
			args:   []string{"container", "update", "e2e-node/200", "swap=0", "sshkeys=ssh-public-key-fixture"},
			shown:  []string{"swap=0", "sshkeys=[REDACTED]"},
			hidden: []string{"ssh-public-key-fixture"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateConfigAndHome(t)
			setupE2EConfig(t)

			var stdout, stderr bytes.Buffer
			err := Run(context.Background(), tt.args, &stdout, &stderr)
			if err == nil {
				t.Fatal("expected update to require confirmation")
			}
			for _, want := range tt.shown {
				if !strings.Contains(stderr.String(), want) || !strings.Contains(err.Error(), want) {
					t.Errorf("confirmation output/error missing %q; stderr=%q error=%v", want, stderr.String(), err)
				}
			}
			for _, secret := range tt.hidden {
				if strings.Contains(stderr.String(), secret) || strings.Contains(err.Error(), secret) {
					t.Errorf("confirmation output/error exposed %q; stderr=%q error=%v", secret, stderr.String(), err)
				}
			}
			if stdout.Len() != 0 {
				t.Fatalf("confirmation preview leaked to stdout: %q", stdout.String())
			}
		})
	}
}
