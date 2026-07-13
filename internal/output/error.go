package output

import (
	"encoding/json"
	"io"

	"github.com/geoffmcc/nodex/internal/redact"
)

// APIError is the structured error envelope returned in JSON mode.
type APIError struct {
	Error  string `json:"error"`
	Detail string `json:"detail,omitempty"`
	Exit   int    `json:"exit"`
}

// WriteErrorJSON writes a structured error response to w.
func WriteErrorJSON(w io.Writer, msg string, detail string, exitCode int) error {
	e := APIError{
		Error:  msg,
		Detail: detail,
		Exit:   exitCode,
	}
	raw, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return err
	}
	redacted := redact.Bytes(raw)
	_, err = w.Write(redacted)
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, "\n")
	return err
}
