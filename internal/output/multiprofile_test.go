package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/geoffmcc/nodex/internal/app"
)

func TestNewMultiProfileOutput(t *testing.T) {
	out := NewMultiProfileOutput[string]()
	if out.Schema != SchemaVersionMultiProfile {
		t.Errorf("Schema = %d, want %d", out.Schema, SchemaVersionMultiProfile)
	}
	if out.Results == nil {
		t.Error("Results is nil, want empty slice")
	}
	if out.Summary.Total != 0 || out.Summary.Success != 0 || out.Summary.Failed != 0 {
		t.Error("Summary should be all zeros")
	}
}

func TestAddSuccess(t *testing.T) {
	out := NewMultiProfileOutput[string]()
	out.AddSuccess("e2e", "hello", 150*time.Millisecond)
	out.AddSuccess("lab", "world", 0)

	if out.Summary.Total != 2 {
		t.Errorf("Total = %d, want 2", out.Summary.Total)
	}
	if out.Summary.Success != 2 {
		t.Errorf("Success = %d, want 2", out.Summary.Success)
	}
	if out.Summary.Failed != 0 {
		t.Errorf("Failed = %d, want 0", out.Summary.Failed)
	}
	if out.Failed() != 0 {
		t.Errorf("Failed() = %d, want 0", out.Failed())
	}
	if out.AllFailed() {
		t.Error("AllFailed() = true, want false")
	}

	r0 := out.Results[0]
	if r0.Profile != "e2e" || !r0.Success || r0.Data != "hello" || r0.Duration != "150ms" || r0.Error != nil {
		t.Errorf("Result[0] = %+v, want profile=e2e success=true data=hello duration=150ms error=nil", r0)
	}
	r1 := out.Results[1]
	if r1.Profile != "lab" || !r1.Success || r1.Data != "world" || r1.Duration != "" || r1.Error != nil {
		t.Errorf("Result[1] = %+v, want profile=lab success=true data=world duration=\"\" error=nil", r1)
	}
}

func TestAddFailure(t *testing.T) {
	out := NewMultiProfileOutput[string]()
	out.AddFailure("bad", fmt.Errorf("connection refused"), 500*time.Millisecond)

	if out.Summary.Total != 1 || out.Summary.Success != 0 || out.Summary.Failed != 1 {
		t.Errorf("Summary = %+v, want total=1 success=0 failed=1", out.Summary)
	}
	if out.Failed() != 1 {
		t.Errorf("Failed() = %d, want 1", out.Failed())
	}
	if !out.AllFailed() {
		t.Error("AllFailed() = false, want true")
	}

	r := out.Results[0]
	if r.Profile != "bad" || r.Success || r.Duration != "500ms" {
		t.Errorf("Result = %+v, want profile=bad success=false duration=500ms", r)
	}
	if r.Error == nil {
		t.Fatal("Error is nil, want ResultError")
	}
	if r.Error.Exit != app.ExitNetwork {
		t.Errorf("Error.Exit = %d, want ExitNetwork (7)", r.Error.Exit)
	}
	if r.Error.Class != "network" {
		t.Errorf("Error.Class = %q, want network", r.Error.Class)
	}
}

func TestAddFailureWithExitCoder(t *testing.T) {
	out := NewMultiProfileOutput[string]()
	authErr := app.NewExitError(fmt.Errorf("401 Unauthorized"), app.ExitAuth)
	out.AddFailure("auth-fail", authErr, 0)

	r := out.Results[0]
	if r.Error.Exit != app.ExitAuth {
		t.Errorf("Error.Exit = %d, want ExitAuth (5)", r.Error.Exit)
	}
	if r.Error.Class != "auth" {
		t.Errorf("Error.Class = %q, want auth", r.Error.Class)
	}
}

func TestSortResults(t *testing.T) {
	out := NewMultiProfileOutput[string]()
	out.AddSuccess("zulu", "z", 0)
	out.AddSuccess("alpha", "a", 0)
	out.AddFailure("mike", fmt.Errorf("err"), 0)
	out.AddSuccess("beta", "b", 0)

	out.SortResults()

	want := []string{"alpha", "beta", "mike", "zulu"}
	for i, name := range want {
		if out.Results[i].Profile != name {
			t.Errorf("Results[%d].Profile = %q, want %q", i, out.Results[i].Profile, name)
		}
	}
}

func TestAllFailed(t *testing.T) {
	// Empty is not all-failed.
	empty := NewMultiProfileOutput[string]()
	if empty.AllFailed() {
		t.Error("empty AllFailed() = true, want false")
	}

	// Partial is not all-failed.
	partial := NewMultiProfileOutput[string]()
	partial.AddSuccess("a", "ok", 0)
	partial.AddFailure("b", fmt.Errorf("err"), 0)
	if partial.AllFailed() {
		t.Error("partial AllFailed() = true, want false")
	}

	// All failures.
	all := NewMultiProfileOutput[string]()
	all.AddFailure("a", fmt.Errorf("err1"), 0)
	all.AddFailure("b", fmt.Errorf("err2"), 0)
	if !all.AllFailed() {
		t.Error("all-fail AllFailed() = false, want true")
	}
}

func TestClassifyToResultError(t *testing.T) {
	// nil returns nil.
	if re := ClassifyToResultError(nil); re != nil {
		t.Errorf("ClassifyToResultError(nil) = %+v, want nil", re)
	}

	// Generic error.
	re := ClassifyToResultError(fmt.Errorf("something went wrong"))
	if re.Exit != app.ExitGeneral || re.Class != "error" {
		t.Errorf("generic: exit=%d class=%q, want exit=1 class=error", re.Exit, re.Class)
	}

	// ProviderError with network issue.
	pe := &app.ProviderError{Err: fmt.Errorf("dial tcp: connection refused")}
	re = ClassifyToResultError(pe)
	if re.Exit != app.ExitNetwork || re.Class != "network" {
		t.Errorf("network: exit=%d class=%q, want exit=7 class=network", re.Exit, re.Class)
	}

	// ExitCoder.
	ec := app.NewExitError(fmt.Errorf("auth failure"), app.ExitAuth)
	re = ClassifyToResultError(ec)
	if re.Exit != app.ExitAuth || re.Class != "auth" {
		t.Errorf("auth: exit=%d class=%q, want exit=5 class=auth", re.Exit, re.Class)
	}
}

func TestMultiProfileOutputJSONRoundTrip(t *testing.T) {
	out := NewMultiProfileOutput[string]()
	out.AddSuccess("e2e", "ok-data", 100*time.Millisecond)
	out.AddFailure("lab", app.NewExitError(fmt.Errorf("timeout"), app.ExitTimeout), 2*time.Second)

	var buf bytes.Buffer
	if err := WriteJSON(&buf, out); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	var decoded MultiProfileOutput[json.RawMessage]
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.Schema != SchemaVersionMultiProfile {
		t.Errorf("Schema = %d, want %d", decoded.Schema, SchemaVersionMultiProfile)
	}
	if len(decoded.Results) != 2 {
		t.Fatalf("Results len = %d, want 2", len(decoded.Results))
	}
	if decoded.Summary.Total != 2 || decoded.Summary.Success != 1 || decoded.Summary.Failed != 1 {
		t.Errorf("Summary = %+v, want total=2 success=1 failed=1", decoded.Summary)
	}
}

func TestErrorClassLabel(t *testing.T) {
	tests := []struct {
		code int
		want string
	}{
		{app.ExitAuth, "auth"},
		{app.ExitAuthorization, "authorization"},
		{app.ExitNetwork, "network"},
		{app.ExitTLS, "tls"},
		{app.ExitTimeout, "timeout"},
		{app.ExitCancellation, "cancellation"},
		{app.ExitNotFound, "not_found"},
		{app.ExitConfig, "config"},
		{app.ExitCredential, "credential"},
		{app.ExitProvider, "provider"},
		{app.ExitValidationError, "validation"},
		{app.ExitConflict, "conflict"},
		{app.ExitRateLimit, "rate_limit"},
		{app.ExitGeneral, "error"},
		{999, "error"},
	}
	for _, tt := range tests {
		got := errorClassLabel(tt.code)
		if got != tt.want {
			t.Errorf("errorClassLabel(%d) = %q, want %q", tt.code, got, tt.want)
		}
	}
}
