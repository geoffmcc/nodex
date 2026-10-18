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

func TestCertificationLedgerMigrationIsLocalAndRequiresYes(t *testing.T) {
	isolateConfigAndHome(t)
	ledger, _, err := certificationLedgerPath(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(ledger), 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := `{"schema":1,"entries":[{"id":"legacy-done","profile":"nodex-test-profile","node":"pve-test","vmid":9005,"name":"nodex-cert-done","storage":"local","state":"cleaned","cleanup":"complete","created_at":5,"updated_at":6}]}`
	if err := os.WriteFile(ledger, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runCertificationLedgerMigration(context.Background(), &Context{}, nil); err == nil {
		t.Fatal("migration without --yes was accepted")
	}
	var out bytes.Buffer
	cmdCtx := &Context{Writer: &out, Opts: Options{Yes: true, Output: output.FormatJSON, Profile: "nodex-test-profile"}}
	if err := runCertificationLedgerMigration(context.Background(), cmdCtx, nil); err != nil {
		t.Fatalf("local migration unexpectedly depended on selected provider profile: %v", err)
	}
	var result struct {
		Status       string `json:"status"`
		Schema       int    `json:"schema"`
		Entries      int    `json:"entries"`
		ProviderUsed bool   `json:"provider_used"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("migration result is not valid JSON: %v", err)
	}
	if result.Status != "migrated" || result.Schema != 2 || result.Entries != 1 || result.ProviderUsed {
		t.Fatalf("migration result = %+v", result)
	}
	backup, err := os.ReadFile(ledger + ".legacy-v1.bak")
	if err != nil || string(backup) != legacy {
		t.Fatalf("migration backup = %q, error = %v", backup, err)
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

func TestCertificationReadonlyIgnoresLegacyLedgerButMutationsFailClosed(t *testing.T) {
	legacy := `{"schema":1,"entries":[{"id":"legacy-entry","profile":"other-profile","node":"pve-other","vmid":9005,"name":"nodex-cert-legacy","storage":"local","state":"created","cleanup":"required","created_at":5,"updated_at":5}]}`
	for _, tt := range []struct {
		name      string
		suite     string
		wantError string
	}{
		{name: "read-only certification", suite: "readonly", wantError: "config file not found"},
		{name: "mutation certification", suite: "disposable-mutations", wantError: "integrity status is legacy_unverified"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			isolateConfigAndHome(t)
			ledger := filepath.Join(t.TempDir(), "ledger.json")
			if tt.suite != "readonly" {
				ledger, _, _ = certificationLedgerPath(nil)
				if err := os.MkdirAll(filepath.Dir(ledger), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(ledger, []byte(legacy), 0o600); err != nil {
				t.Fatal(err)
			}
			name := "nodex-cert-ledger-check"
			cmdCtx := &Context{Opts: Options{
				Profile: "nodex-test-profile", Yes: true, Force: true, ConfirmTarget: name,
			}}
			args := []string{
				"--ledger", ledger,
				"--environment", "lab", "--suite", tt.suite,
				"--node", "pve-test", "--vmid", "9006",
				"--name", name, "--storage", "local",
			}
			err := runCertification(context.Background(), cmdCtx, args)
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("runCertification(%s) error = %v, want %q", tt.suite, err, tt.wantError)
			}
		})
	}
}

func TestCertificationMutationRejectsAlternateLedger(t *testing.T) {
	isolateConfigAndHome(t)
	cmdCtx := &Context{Opts: Options{
		Profile: "nodex-test-profile", Yes: true, Force: true,
		ConfirmTarget: "nodex-cert-accounting",
	}}
	args := []string{
		"--ledger", filepath.Join(t.TempDir(), "alternate.json"),
		"--environment", "lab", "--suite", "disposable-mutations",
		"--node", "pve-test", "--vmid", "9006",
		"--name", "nodex-cert-accounting", "--storage", "local",
	}
	err := runCertification(context.Background(), cmdCtx, args)
	if err == nil || !strings.Contains(err.Error(), "must use the default per-configuration ledger") {
		t.Fatalf("runCertification error = %v, want alternate-ledger rejection before configuration or connection", err)
	}
}
