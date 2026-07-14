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

// TestSentinelErrors verifies all sentinel error values exist and are distinct.
func TestSentinelErrors(t *testing.T) {
	errors := []error{
		ErrAuthorizationRequired,
		ErrNonInteractiveRequired,
		ErrExpertRequired,
		ErrTypeConfirmMismatch,
	}
	for i, e1 := range errors {
		for j, e2 := range errors {
			if i != j && e1 == e2 {
				t.Errorf("sentinel errors at [%d] and [%d] are identical", i, j)
			}
		}
		if e1.Error() == "" {
			t.Errorf("sentinel error [%d] has empty message", i)
		}
	}
}

// TestCheckReturnsConfirmationRequired verifies that Check correctly identifies
// when confirmation is needed for each tier.
func TestCheckReturnsConfirmationRequired(t *testing.T) {
	tests := []struct {
		name     string
		policy   ConfirmationPolicy
		yes      bool
		force    bool
		wantConf bool
	}{
		{name: "observation never needs confirmation", policy: ConfirmationPolicy{Tier: TierObservation}, wantConf: false},
		{name: "reversible without yes needs confirmation", policy: ConfirmationPolicy{Tier: TierReversible}, wantConf: true},
		{name: "reversible with yes is authorized", policy: ConfirmationPolicy{Tier: TierReversible}, yes: true, wantConf: false},
		{name: "disruptive without flags needs confirmation", policy: ConfirmationPolicy{Tier: TierDisruptive}, wantConf: true},
		{name: "disruptive with yes only needs confirmation", policy: ConfirmationPolicy{Tier: TierDisruptive}, yes: true, wantConf: true},
		{name: "disruptive with yes+force is authorized", policy: ConfirmationPolicy{Tier: TierDisruptive}, yes: true, force: true, wantConf: false},
		{name: "destructive type-confirm always needs confirmation", policy: ConfirmationPolicy{Tier: TierDestructive, RequiresTypeConfirm: true, TypeConfirmTarget: "t"}, yes: true, force: true, wantConf: true},
		{name: "destructive no type-confirm with flags authorized", policy: ConfirmationPolicy{Tier: TierDestructive}, yes: true, force: true, wantConf: false},
		{name: "security admin without flags needs confirmation", policy: ConfirmationPolicy{Tier: TierSecurityAdmin}, wantConf: true},
		{name: "security admin with flags authorized", policy: ConfirmationPolicy{Tier: TierSecurityAdmin}, yes: true, force: true, wantConf: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.policy.Check(tt.yes, tt.force, false)
			if result.ConfirmationRequired != tt.wantConf {
				t.Errorf("ConfirmationRequired = %v, want %v", result.ConfirmationRequired, tt.wantConf)
			}
		})
	}
}

// TestNonInteractiveBlocksAllTiers verifies non-interactive mode blocks all tiers.
func TestNonInteractiveBlocksAllTiers(t *testing.T) {
	policies := []struct {
		name   string
		policy ConfirmationPolicy
		yes    bool
		force  bool
	}{
		{name: "reversible without yes", policy: ConfirmationPolicy{Tier: TierReversible}},
		{name: "disruptive without force", policy: ConfirmationPolicy{Tier: TierDisruptive}, yes: true},
		{name: "destructive type-confirm", policy: ConfirmationPolicy{Tier: TierDestructive, RequiresTypeConfirm: true, TypeConfirmTarget: "t"}, yes: true, force: true},
		{name: "security admin without flags", policy: ConfirmationPolicy{Tier: TierSecurityAdmin}},
	}
	for _, tt := range policies {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.policy.Check(tt.yes, tt.force, true)
			if !result.ConfirmationRequired {
				t.Error("non-interactive should require confirmation when flags insufficient")
			}
		})
	}
}
