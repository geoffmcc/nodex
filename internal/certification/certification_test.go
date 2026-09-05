package certification

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateRequestIsFailClosed(t *testing.T) {
	base := Request{Profile: RequiredProfile, Node: "pve-test", VMID: 9000, Name: "nodex-cert-smoke", Storage: "local", ConfirmTarget: "nodex-cert-smoke", OptIn: true}
	tests := []struct {
		name   string
		change func(*Request)
		want   string
	}{
		{"requires opt in", func(r *Request) { r.OptIn = false }, "opt-in"},
		{"requires test profile", func(r *Request) { r.Profile = "production" }, "nodex-test-admin"},
		{"requires prefix", func(r *Request) { r.Name = "smoke" }, NamePrefix},
		{"requires exact confirmation", func(r *Request) { r.ConfirmTarget = "nodex-cert-other" }, "exactly"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := base
			tt.change(&r)
			if err := ValidateRequest(r); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestValidateAuthorizationRequiresExactBinding(t *testing.T) {
	r := Request{Profile: RequiredProfile, Environment: "lab", Suite: "readonly", Endpoint: "https://pve.example.test", Node: "pve1", VMID: 9001, Storage: "local", Authorization: &Authorization{Environment: "lab", Profile: RequiredProfile, Endpoint: "https://pve.example.test", Provider: "proxmox", ExpectedFingerprint: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", TrustedCAIdentity: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Nodes: []string{"pve1"}, Storage: []string{"local"}, VMIDMin: 9000, VMIDMax: 9099, Suites: []string{"readonly"}, MaxResources: 1, ExpiresAt: time.Now().Add(time.Hour).Unix()}}
	if err := ValidateAuthorization(r); err != nil {
		t.Fatalf("valid authorization rejected: %v", err)
	}
	r.Node = "other-node"
	if err := ValidateAuthorization(r); err == nil {
		t.Fatal("unauthorized node accepted")
	}
}

func TestLedgerRoundTripIsSanitizedAndSorted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")
	l := New(path)
	l.Entries = append(l.Entries,
		NewEntry(RequiredProfile, "pve-test", 9002, "nodex-cert-b", "local", time.Unix(2, 0)),
		NewEntry(RequiredProfile, "pve-test", 9001, "nodex-cert-a", "local", time.Unix(1, 0)),
	)
	if err := Save(path, l); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Entries) != 2 || loaded.Entries[0].Name != "nodex-cert-a" {
		t.Fatalf("unexpected ledger ordering: %+v", loaded.Entries)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "secret") || strings.Contains(string(b), "credential") {
		t.Fatalf("ledger contains credential material: %s", b)
	}
}
