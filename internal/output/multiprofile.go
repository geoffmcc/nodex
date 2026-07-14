package output

import (
	"sort"
	"time"

	"github.com/geoffmcc/nodex/internal/app"
)

// SchemaVersionMultiProfile is the current schema version for MultiProfileOutput.
// Increment when backwards-incompatible changes are made to the envelope shape.
const SchemaVersionMultiProfile = 1

// MultiProfileOutput is the standard envelope for multi-profile command results.
// It wraps per-profile results with aggregate summary statistics so that
// automation consumers can inspect the outcome of every profile, classify
// failures per-profile, and determine the overall exit code.
type MultiProfileOutput[T any] struct {
	Schema  int                 `json:"schema" yaml:"schema"`
	Results []ProfileResult[T]  `json:"results" yaml:"results"`
	Summary MultiProfileSummary `json:"summary" yaml:"summary"`
}

// ProfileResult carries a per-profile outcome: the profile name, success
// status, typed payload (inspection data, entities, UPIDs, etc.), and a
// classified error when the profile failed.  Duration records the wall-clock
// time spent on this profile (including connection, fetch, and cleanup).
type ProfileResult[T any] struct {
	Profile  string       `json:"profile" yaml:"profile"`
	Success  bool         `json:"success" yaml:"success"`
	Data     T            `json:"data,omitempty" yaml:"data,omitempty"`
	Error    *ResultError `json:"error,omitempty" yaml:"error,omitempty"`
	Duration string       `json:"duration,omitempty" yaml:"duration,omitempty"`
}

// MultiProfileSummary provides aggregate statistics across all profiles.
type MultiProfileSummary struct {
	Total   int `json:"total" yaml:"total"`
	Success int `json:"success" yaml:"success"`
	Failed  int `json:"failed" yaml:"failed"`
}

// NewMultiProfileOutput creates an initialized empty envelope.
func NewMultiProfileOutput[T any]() MultiProfileOutput[T] {
	return MultiProfileOutput[T]{
		Schema:  SchemaVersionMultiProfile,
		Results: make([]ProfileResult[T], 0),
	}
}

// AddSuccess adds a successful profile result with timing.
func (m *MultiProfileOutput[T]) AddSuccess(profile string, data T, d time.Duration) {
	m.Results = append(m.Results, ProfileResult[T]{
		Profile:  profile,
		Success:  true,
		Data:     data,
		Duration: formatMillis(d),
	})
	m.Summary.Success++
	m.Summary.Total++
}

// AddFailure adds a failed profile result with classified error and timing.
func (m *MultiProfileOutput[T]) AddFailure(profile string, err error, d time.Duration) {
	m.Results = append(m.Results, ProfileResult[T]{
		Profile:  profile,
		Success:  false,
		Error:    ClassifyToResultError(err),
		Duration: formatMillis(d),
	})
	m.Summary.Failed++
	m.Summary.Total++
}

// SortResults sorts results alphabetically by profile name for deterministic output.
func (m *MultiProfileOutput[T]) SortResults() {
	sort.Slice(m.Results, func(i, j int) bool {
		return m.Results[i].Profile < m.Results[j].Profile
	})
}

// Failed returns the number of failed profiles.
func (m *MultiProfileOutput[T]) Failed() int { return m.Summary.Failed }

// AllFailed returns true when every attempted profile failed.
func (m *MultiProfileOutput[T]) AllFailed() bool {
	return m.Summary.Total > 0 && m.Summary.Failed == m.Summary.Total
}

// ClassifyToResultError converts any error to a structured ResultError with
// classified exit code and human-readable class label.  The detail string is
// redacted by WriteJSON/WriteYAML through the normal redact.Sanitize path.
func ClassifyToResultError(err error) *ResultError {
	if err == nil {
		return nil
	}
	code := app.ExitCodeFromError(err)
	return &ResultError{
		Class:  errorClassLabel(code),
		Exit:   code,
		Detail: err.Error(),
	}
}

// errorClassLabel maps exit codes to short, stable, machine-readable labels.
func errorClassLabel(code int) string {
	switch code {
	case app.ExitAuth:
		return "auth"
	case app.ExitAuthorization:
		return "authorization"
	case app.ExitNetwork:
		return "network"
	case app.ExitTLS:
		return "tls"
	case app.ExitTimeout:
		return "timeout"
	case app.ExitCancellation:
		return "cancellation"
	case app.ExitNotFound:
		return "not_found"
	case app.ExitConfig:
		return "config"
	case app.ExitCredential:
		return "credential"
	case app.ExitProvider:
		return "provider"
	case app.ExitValidationError:
		return "validation"
	case app.ExitConflict:
		return "conflict"
	case app.ExitRateLimit:
		return "rate_limit"
	default:
		return "error"
	}
}

func formatMillis(d time.Duration) string {
	if d == 0 {
		return ""
	}
	return d.Round(time.Millisecond).String()
}
