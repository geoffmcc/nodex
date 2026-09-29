package output

import (
	"io"

	"gopkg.in/yaml.v3"

	"github.com/geoffmcc/nodex/internal/redact"
)

// APIError is the structured error envelope for pre-flight and other failures
// that occur instead of — or before — a mutation result envelope.
//
// Its `error` key carries the same *ResultError object that OperationResult
// uses, so a consumer can read doc.error.class, doc.error.exit and
// doc.error.detail identically on every path. It previously carried a bare
// string, which made the `error` key polymorphic — a string here and an object
// on the mutation path — so that every consumer had to branch on its type
// before it could read it, and the naive handler worked on the paths it was
// tested against and then failed in production.
type APIError struct {
	// Schema is the schema version for forward-compatibility, matching
	// OperationResult so both envelopes are self-describing.
	Schema int `json:"schema" yaml:"schema"`

	// Error carries the classified error. Always an object, never a string.
	Error *ResultError `json:"error" yaml:"error"`

	// Exit repeats the process exit code at the top level for convenience.
	Exit int `json:"exit" yaml:"exit"`
}

// NewAPIError builds an APIError from a message and exit code, deriving the
// machine-readable class from the exit code so that callers of the plain error
// path get the same taxonomy as callers of the mutation path.
//
// detail is optional supporting context; when present it is folded into the
// single canonical prose location (error.detail) rather than being exposed as
// a second, ambiguous "detail" key at the top level.
func NewAPIError(msg, detail string, exitCode int) APIError {
	full := msg
	if detail != "" {
		full = msg + ": " + detail
	}
	return APIError{
		Schema: SchemaVersionResult,
		Error: &ResultError{
			Class:  errorClassLabel(exitCode),
			Exit:   exitCode,
			Detail: full,
		},
		Exit: exitCode,
	}
}

// WriteErrorJSON writes the structured error envelope as JSON to w.
func WriteErrorJSON(w io.Writer, msg string, detail string, exitCode int) error {
	return writeErrorEnvelope(w, FormatJSON, msg, detail, exitCode)
}

// WriteErrorYAML mirrors WriteErrorJSON for --output yaml. Without it a YAML
// consumer received a bare "Error: ..." prose line with no exit code and no
// structure, and had to scrape text to learn what went wrong.
func WriteErrorYAML(w io.Writer, msg string, detail string, exitCode int) error {
	return writeErrorEnvelope(w, FormatYAML, msg, detail, exitCode)
}

func writeErrorEnvelope(w io.Writer, format Format, msg, detail string, exitCode int) error {
	sanitized := sanitizeTerminalData(redact.Sanitize(NewAPIError(msg, detail, exitCode)))

	var raw []byte
	var err error
	if format == FormatYAML {
		raw, err = yaml.Marshal(sanitized)
	} else {
		raw, err = marshalIndentNoEscape(sanitized)
	}
	if err != nil {
		return err
	}
	if _, err := w.Write(raw); err != nil {
		return err
	}
	if format == FormatYAML {
		return nil
	}
	_, err = io.WriteString(w, "\n")
	return err
}
