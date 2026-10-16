package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRequireCertificationProfileRequiresExplicitSelection(t *testing.T) {
	cmdCtx := &Context{}
	if err := requireCertificationProfile(cmdCtx); err == nil {
		t.Fatal("missing explicit profile was accepted")
	}
	cmdCtx.Opts.Profile = "nodex-test-profile"
	if err := requireCertificationProfile(cmdCtx); err != nil {
		t.Fatalf("explicit lab profile rejected: %v", err)
	}
}

func TestCertificationReportShowsEmptyLedgerAsNoRuns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")
	if err := os.WriteFile(path, []byte(`{"schema":1,"entries":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmdCtx := &Context{Writer: &out}
	if err := runCertificationReport(context.Background(), cmdCtx, []string{"--ledger", path}); err != nil {
		t.Fatalf("runCertificationReport: %v", err)
	}
	if got, want := out.String(), "No certification runs recorded.\n"; got != want {
		t.Fatalf("report = %q, want %q", got, want)
	}
}
