package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/output"
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

func TestCertificationReportMarksLegacyLedgerUnverified(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")
	legacy := `{"schema":1,"entries":[{"id":"legacy-entry","profile":"nodex-test-profile","node":"pve-test","vmid":9005,"name":"nodex-cert-legacy","storage":"local","state":"created","cleanup":"required","created_at":5,"updated_at":5}]}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmdCtx := &Context{Writer: &out, Opts: Options{Output: output.FormatJSON}}
	if err := runCertificationReport(context.Background(), cmdCtx, []string{"--ledger", path}); err != nil {
		t.Fatalf("runCertificationReport: %v", err)
	}
	var report struct {
		IntegrityStatus string `json:"integrity_status"`
		Entries         []any  `json:"entries"`
	}
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("report is not valid JSON: %v", err)
	}
	if report.IntegrityStatus != "legacy_unverified" || len(report.Entries) != 1 {
		t.Fatalf("report status/entries = %q/%d, want legacy_unverified/1", report.IntegrityStatus, len(report.Entries))
	}
}

func TestCertificationReportScopesEntriesToExplicitProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")
	legacy := `{"schema":1,"entries":[{"id":"lab-entry","profile":"nodex-test-profile","node":"pve-test","vmid":9005,"name":"nodex-cert-lab","storage":"local","state":"created","cleanup":"required","created_at":5,"updated_at":5},{"id":"other-entry","profile":"other-profile","node":"pve-other","vmid":9006,"name":"nodex-cert-other","storage":"local","state":"created","cleanup":"required","created_at":6,"updated_at":6}]}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmdCtx := &Context{Writer: &out, Opts: Options{Profile: "nodex-test-profile", Output: output.FormatJSON}}
	if err := runCertificationReport(context.Background(), cmdCtx, []string{"--ledger", path}); err != nil {
		t.Fatalf("runCertificationReport: %v", err)
	}
	var report struct {
		Entries []struct {
			ID string `json:"id"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("report is not valid JSON: %v", err)
	}
	if len(report.Entries) != 1 || report.Entries[0].ID != "lab-entry" {
		t.Fatalf("report entries = %#v, want only lab-entry", report.Entries)
	}
	if strings.Contains(out.String(), "other-entry") || strings.Contains(out.String(), "pve-other") {
		t.Fatalf("report leaked an entry from another profile: %s", out.String())
	}
}

func TestCertificationCleanupRejectsLegacyLedgerBeforeConnecting(t *testing.T) {
	isolateConfigAndHome(t)
	path := filepath.Join(t.TempDir(), "ledger.json")
	legacy := `{"schema":1,"entries":[{"id":"legacy-entry","profile":"nodex-test-profile","node":"pve-test","vmid":9005,"name":"nodex-cert-legacy","storage":"local","state":"created","cleanup":"required","created_at":5,"updated_at":5}]}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	cmdCtx := &Context{Opts: Options{Profile: "nodex-test-profile", Yes: true, ConfirmTarget: "legacy-entry"}}
	err := runCertificationCleanup(context.Background(), cmdCtx, []string{"--ledger", path})
	if err == nil || !strings.Contains(err.Error(), "integrity status is legacy_unverified") {
		t.Fatalf("runCertificationCleanup error = %v, want legacy-integrity block", err)
	}
}
