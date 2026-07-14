// Package safety provides mutation safety classification, confirmation policy,
// and dry-run support for Nodex operations.
package safety

import (
	"errors"
	"fmt"
)

// Sentinel errors for safety authorization outcomes.
var (
	// ErrAuthorizationRequired is returned when interactive confirmation
	// is required but has not been provided.
	ErrAuthorizationRequired = errors.New("authorization required")

	// ErrNonInteractiveRequired is returned when interactive confirmation
	// is needed but the session is non-interactive.
	ErrNonInteractiveRequired = errors.New("interactive confirmation required in non-interactive mode")

	// ErrExpertRequired is returned when a Tier 4 (SecurityAdmin) operation
	// is attempted without the --expert flag.
	ErrExpertRequired = errors.New("expert mode required for security administration")

	// ErrTypeConfirmMismatch is returned when the user's typed confirmation
	// does not match the required target.
	ErrTypeConfirmMismatch = errors.New("type confirmation mismatch")
)

// Tier classifies the risk level of an operation.
type Tier int

const (
	// TierObservation is read-only; no confirmation required.
	TierObservation Tier = 0

	// TierReversible is a reversible state change (e.g., start VM).
	// Requires --yes or interactive confirmation.
	TierReversible Tier = 1

	// TierDisruptive affects running workloads (e.g., reset VM, migrate).
	// Requires --yes --force or double confirmation.
	TierDisruptive Tier = 2

	// TierDestructive causes permanent data loss (e.g., delete VM, destroy disk).
	// Requires type-in target verification.
	TierDestructive Tier = 3

	// TierSecurityAdmin changes identity/ACL state.
	// Requires explicit expert-mode opt-in.
	TierSecurityAdmin Tier = 4
)

// String returns the tier name.
func (t Tier) String() string {
	switch t {
	case TierObservation:
		return "observation"
	case TierReversible:
		return "reversible"
	case TierDisruptive:
		return "disruptive"
	case TierDestructive:
		return "destructive"
	case TierSecurityAdmin:
		return "security_admin"
	default:
		return fmt.Sprintf("unknown(%d)", t)
	}
}

// ConfirmationPolicy defines the safety requirements for an operation.
type ConfirmationPolicy struct {
	// Tier is the safety tier of this operation.
	Tier Tier

	// ResourceDescription is a human-readable description shown in prompts
	// (e.g., "VM pve1/100 (webserver-prod)").
	ResourceDescription string

	// RequiresTypeConfirm enables type-in confirmation for Tier 3+.
	// The user must type the explicit target identifier.
	RequiresTypeConfirm bool

	// TypeConfirmTarget is the exact string the user must type to confirm.
	// Only used when RequiresTypeConfirm is true.
	TypeConfirmTarget string
}

// ConfirmationResult represents the outcome of a confirmation check.
type ConfirmationResult struct {
	// ConfirmationRequired is true when the user must confirm before proceeding.
	ConfirmationRequired bool

	// TypeConfirmRequired is true when Tier 3 type-in verification is needed.
	TypeConfirmRequired bool

	// DoubleConfirmRequired is true when Tier 2+ needs --force after --yes.
	DoubleConfirmRequired bool

	// Message is a human-readable prompt to display to the user.
	Message string

	// Warning provides additional caution text for disruptive/destructive ops.
	Warning string
}

// Check determines what confirmation, if any, is required for an operation
// given the provided flags. It returns a ConfirmationResult describing what
// the user must do to proceed. Callers in non-interactive mode should check
// flags; interactive mode should prompt on stderr.
//
// The yes flag acknowledges basic confirmation (Tier 1+).
// The force flag acknowledges double confirmation (Tier 2+).
// The nonInteractive flag suppresses prompts; the caller must fail if
// confirmation is required and not already provided via flags.
func (p ConfirmationPolicy) Check(yes, force, nonInteractive bool) ConfirmationResult {
	r := ConfirmationResult{}

	switch {
	case p.Tier == TierObservation:
		// No confirmation for read-only operations.
		return r

	case p.Tier == TierReversible:
		if yes {
			return r
		}
		r.ConfirmationRequired = true
		r.Message = p.confirmationMessage()
		if nonInteractive {
			r.Message += " Use --yes to confirm."
		}
		return r

	case p.Tier == TierDisruptive:
		if yes && force {
			return r
		}
		r.ConfirmationRequired = true
		r.DoubleConfirmRequired = !force || !yes
		r.Warning = "This operation is disruptive and may affect running workloads."
		r.Message = p.confirmationMessage()
		if !yes {
			r.Message += " Use --yes to confirm."
		}
		if yes && !force {
			r.Message += " Use --force for double confirmation."
		}
		return r

	case p.Tier >= TierDestructive:
		if p.RequiresTypeConfirm {
			if yes && force {
				// Type confirmation still required even with flags.
				r.ConfirmationRequired = true
				r.TypeConfirmRequired = true
				r.DoubleConfirmRequired = false // flags handled yes+force
				r.Warning = "This operation is destructive and cannot be undone."
				r.Message = fmt.Sprintf("Type %q to confirm: ", p.TypeConfirmTarget)
				return r
			}
			r.ConfirmationRequired = true
			r.DoubleConfirmRequired = !force || !yes
			r.TypeConfirmRequired = true
			r.Warning = "This operation is destructive and cannot be undone. " +
				"Consider creating a backup first."
			r.Message = p.confirmationMessage()
			if !yes {
				r.Message += " Use --yes to confirm."
			}
			if yes && !force {
				r.Message += " Use --force for double confirmation."
			}
			r.Message += fmt.Sprintf(" Then type %q to confirm.", p.TypeConfirmTarget)
			return r
		}

		// Tier 3+ without type confirmation is still treated as disruptive+.
		if yes && force {
			return r
		}
		r.ConfirmationRequired = true
		r.DoubleConfirmRequired = !force || !yes
		r.Warning = "This operation is destructive and cannot be undone."
		r.Message = p.confirmationMessage()
		if !yes {
			r.Message += " Use --yes to confirm."
		}
		if yes && !force {
			r.Message += " Use --force for double confirmation."
		}
		return r
	}

	return r
}

// confirmationMessage builds the basic resource description message.
func (p ConfirmationPolicy) confirmationMessage() string {
	if p.ResourceDescription != "" {
		return fmt.Sprintf("Operation on %s.", p.ResourceDescription)
	}
	return "Proceed with operation?"
}

// MustConfirm is a convenience method that returns true if any confirmation
// (including type-in) is required for the given flags. This is useful for
// quick pre-flight checks.
func (p ConfirmationPolicy) MustConfirm(yes, force bool) bool {
	return p.Check(yes, force, false).ConfirmationRequired
}

// DryRun checks if this is a dry-run operation (always Tier 0).
type DryRun bool

// IsDryRun returns true if this is a dry run.
func (d DryRun) IsDryRun() bool { return bool(d) }

// NewDryRun creates a dry-run context marker.
func NewDryRun() DryRun { return DryRun(true) }

// NewLive creates a live-operation context marker.
func NewLive() DryRun { return DryRun(false) }
