package output

import (
	"fmt"
	"io"
)

// SchemaVersionResult is the current schema version for OperationResult.
// Increment when backwards-incompatible changes are made to the JSON/YAML shape.
const SchemaVersionResult = 1

// ResultError carries classified error information in structured output.
type ResultError struct {
	Class  string `json:"class,omitempty" yaml:"class,omitempty"`
	Exit   int    `json:"exit" yaml:"exit"`
	Detail string `json:"detail,omitempty" yaml:"detail,omitempty"`
}

// OperationResult is the standard result envelope for state-changing commands.
// It provides human and machine consumers with a single, predictable
// representation of every mutation, whether synchronous or asynchronous, with
// or without --wait.
type OperationResult struct {
	// Schema is the schema version for forward-compatibility.
	Schema int `json:"schema" yaml:"schema"`

	// Operation is the human-readable command name (e.g., "vm start").
	Operation string `json:"operation" yaml:"operation"`

	// Profile is the configuration profile name used.
	Profile string `json:"profile,omitempty" yaml:"profile,omitempty"`

	// Provider is the provider backend name (e.g., "proxmox").
	Provider string `json:"provider" yaml:"provider"`

	// Target identifies the resource operated on.
	Target string `json:"target,omitempty" yaml:"target,omitempty"`

	// Safety is the safety tier label (e.g., "reversible", "destructive").
	Safety string `json:"safety,omitempty" yaml:"safety,omitempty"`

	// UPID is the provider-side task identifier when one was returned.
	UPID string `json:"upid,omitempty" yaml:"upid,omitempty"`

	// Submitted is true when the provider accepted the request.
	Submitted bool `json:"submitted" yaml:"submitted"`

	// Waited is true when Nodex waited for the provider task to complete.
	Waited bool `json:"waited" yaml:"waited"`

	// Success is true when the overall operation was successful.
	// For --wait operations this mirrors the provider task result.
	// For non-wait operations this means the request was accepted.
	Success bool `json:"success" yaml:"success"`

	// Changed indicates whether state was modified. nil when unknowable.
	Changed *bool `json:"changed,omitempty" yaml:"changed,omitempty"`

	// Status is a provider-defined status string (e.g., "OK" for Proxmox tasks).
	Status string `json:"status,omitempty" yaml:"status,omitempty"`

	// Warnings holds human-readable warnings for the operation.
	Warnings []string `json:"warnings,omitempty" yaml:"warnings,omitempty"`

	// Error carries classified error details when success is false.
	Error *ResultError `json:"error,omitempty" yaml:"error,omitempty"`
}

// NewOperationResult creates a baseline result with schema version and
// operation metadata populated.
func NewOperationResult(operation, provider, profile string) OperationResult {
	return OperationResult{
		Schema:    SchemaVersionResult,
		Operation: operation,
		Provider:  provider,
		Profile:   profile,
	}
}

// WriteResult writes an OperationResult to w in the requested format.
// Table output is a concise single-line summary. JSON and YAML output
// include the full structured envelope.
func WriteResult(w io.Writer, format Format, result OperationResult) error {
	switch format {
	case FormatJSON:
		return WriteJSON(w, result)
	case FormatYAML:
		return WriteYAML(w, result)
	default:
		return writeResultTable(w, result)
	}
}

// writeResultTable formats a single OperationResult as a concise text line.
func writeResultTable(w io.Writer, r OperationResult) error {
	switch {
	case r.Error != nil && !r.Success:
		if r.UPID != "" {
			return writeLine(w, fmt.Sprintf("Task %s FAILED for %s on %s: %s",
				r.UPID, r.Operation, r.Target, r.Error.Detail))
		}
		return writeLine(w, fmt.Sprintf("%s on %s FAILED: %s",
			r.Operation, r.Target, r.Error.Detail))
	case r.Waited && r.Success:
		return writeLine(w, fmt.Sprintf("Task %s completed OK for %s on %s",
			r.UPID, r.Operation, r.Target))
	case r.UPID != "" && r.Submitted:
		return writeLine(w, fmt.Sprintf("Submitted task %s for %s on %s",
			r.UPID, r.Operation, r.Target))
	default:
		// Non-UPID successful mutations.
		return writeLine(w, fmt.Sprintf("%s on %s completed", r.Operation, r.Target))
	}
}

// writeLine writes a redacted, newline-terminated string and returns any write error.
func writeLine(w io.Writer, s string) error {
	clean := SanitizeTerminal(s)
	_, err := fmt.Fprintf(w, "%s\n", clean)
	return err
}
