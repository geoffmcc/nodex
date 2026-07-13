package safety

import (
	"testing"
)

func TestTierString(t *testing.T) {
	tests := []struct {
		tier Tier
		want string
	}{
		{TierObservation, "observation"},
		{TierReversible, "reversible"},
		{TierDisruptive, "disruptive"},
		{TierDestructive, "destructive"},
		{TierSecurityAdmin, "security_admin"},
	}
	for _, tt := range tests {
		if got := tt.tier.String(); got != tt.want {
			t.Errorf("Tier(%d).String() = %q, want %q", tt.tier, got, tt.want)
		}
	}
}

func TestObservationNoConfirmation(t *testing.T) {
	policy := ConfirmationPolicy{
		Tier:                TierObservation,
		ResourceDescription: "list VMs",
	}
	result := policy.Check(false, false, true)
	if result.ConfirmationRequired {
		t.Error("observation should not require confirmation")
	}
}

func TestReversibleRequiresYes(t *testing.T) {
	policy := ConfirmationPolicy{
		Tier:                TierReversible,
		ResourceDescription: "VM pve1/100",
	}

	// Without --yes, confirmation required.
	result := policy.Check(false, false, false)
	if !result.ConfirmationRequired {
		t.Error("reversible without --yes should require confirmation")
	}

	// With --yes, no confirmation required.
	result = policy.Check(true, false, false)
	if result.ConfirmationRequired {
		t.Error("reversible with --yes should not require confirmation")
	}
}

func TestReversibleNonInteractive(t *testing.T) {
	policy := ConfirmationPolicy{
		Tier:                TierReversible,
		ResourceDescription: "VM pve1/100",
	}
	result := policy.Check(false, false, true)
	if !result.ConfirmationRequired {
		t.Error("reversible in non-interactive without --yes should require confirmation")
	}
	if !contains(result.Message, "--yes") {
		t.Errorf("message should mention --yes: %q", result.Message)
	}

	result = policy.Check(true, false, true)
	if result.ConfirmationRequired {
		t.Error("reversible with --yes in non-interactive should not require confirmation")
	}
}

func TestDisruptiveRequiresYesAndForce(t *testing.T) {
	policy := ConfirmationPolicy{
		Tier:                TierDisruptive,
		ResourceDescription: "VM reset pve1/100",
	}

	// Without flags.
	result := policy.Check(false, false, false)
	if !result.ConfirmationRequired {
		t.Error("disruptive without flags should require confirmation")
	}
	if !result.DoubleConfirmRequired {
		t.Error("disruptive should require double confirm")
	}

	// With --yes only.
	result = policy.Check(true, false, false)
	if !result.ConfirmationRequired {
		t.Error("disruptive with --yes only should require confirmation")
	}
	if !result.DoubleConfirmRequired {
		t.Error("disruptive with --yes only should require double confirm")
	}

	// With --yes --force.
	result = policy.Check(true, true, false)
	if result.ConfirmationRequired {
		t.Error("disruptive with --yes --force should not require confirmation")
	}
}

func TestDisruptiveWarning(t *testing.T) {
	policy := ConfirmationPolicy{
		Tier:                TierDisruptive,
		ResourceDescription: "VM reset pve1/100",
	}
	result := policy.Check(false, false, false)
	if result.Warning == "" {
		t.Error("disruptive should have a warning message")
	}
}

func TestDestructiveRequiresTypeConfirm(t *testing.T) {
	policy := ConfirmationPolicy{
		Tier:                TierDestructive,
		ResourceDescription: "VM delete pve1/100",
		RequiresTypeConfirm: true,
		TypeConfirmTarget:   "pve1/100",
	}

	// Without flags.
	result := policy.Check(false, false, false)
	if !result.ConfirmationRequired {
		t.Error("destructive without flags should require confirmation")
	}
	if !result.TypeConfirmRequired {
		t.Error("destructive should require type confirmation")
	}

	// With --yes --force, still needs type confirmation.
	result = policy.Check(true, true, false)
	if !result.ConfirmationRequired {
		t.Error("destructive with flags should still require type confirmation")
	}
	if !result.TypeConfirmRequired {
		t.Error("destructive with flags should still require type confirmation")
	}
	if !contains(result.Message, "pve1/100") {
		t.Errorf("message should contain TypeConfirmTarget: %q", result.Message)
	}
}

func TestDestructiveWarning(t *testing.T) {
	policy := ConfirmationPolicy{
		Tier:                TierDestructive,
		ResourceDescription: "VM delete pve1/100",
		RequiresTypeConfirm: true,
		TypeConfirmTarget:   "pve1/100",
	}
	result := policy.Check(false, false, false)
	if result.Warning == "" {
		t.Error("destructive should have a warning message")
	}
}

func TestMustConfirm(t *testing.T) {
	policy := ConfirmationPolicy{Tier: TierObservation}
	if policy.MustConfirm(false, false) {
		t.Error("observation should not must-confirm")
	}

	policy = ConfirmationPolicy{Tier: TierReversible}
	if !policy.MustConfirm(false, false) {
		t.Error("reversible should must-confirm without --yes")
	}
	if policy.MustConfirm(true, false) {
		t.Error("reversible should not must-confirm with --yes")
	}

	policy = ConfirmationPolicy{Tier: TierDisruptive}
	if !policy.MustConfirm(false, false) {
		t.Error("disruptive should must-confirm without flags")
	}
	if policy.MustConfirm(true, true) {
		t.Error("disruptive should not must-confirm with --yes --force")
	}
}

func TestDryRun(t *testing.T) {
	d := NewDryRun()
	if !d.IsDryRun() {
		t.Error("NewDryRun should be dry run")
	}

	l := NewLive()
	if l.IsDryRun() {
		t.Error("NewLive should not be dry run")
	}
}

func TestConfirmationMessageWithResource(t *testing.T) {
	policy := ConfirmationPolicy{
		Tier:                TierReversible,
		ResourceDescription: "VM pve1/100 (webserver)",
	}
	result := policy.Check(false, false, false)
	if !contains(result.Message, "VM pve1/100 (webserver)") {
		t.Errorf("message should contain resource description: %q", result.Message)
	}
}

func TestConfirmationMessageWithoutResource(t *testing.T) {
	policy := ConfirmationPolicy{Tier: TierReversible}
	result := policy.Check(false, false, false)
	if !contains(result.Message, "Proceed with operation?") {
		t.Errorf("message should have default prompt: %q", result.Message)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}
