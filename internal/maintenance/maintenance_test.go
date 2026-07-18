package maintenance

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/geoffmcc/nodex/internal/ansible"
)

var fixedNow = time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)

func samplePlan(t *testing.T) Plan {
	t.Helper()
	hosts := []PlanHost{
		{Name: "web1", Address: "web1.example.invalid", Role: "generic", Criticality: "standard",
			PendingUpdates: []string{"nano", "openssl"}, SecurityUpdates: []string{"openssl"}},
		{Name: "pve-primary", Address: "pve.example.invalid", Role: "pve", Criticality: "critical",
			BackupRequired: true, RebootRequired: true},
		{Name: "dns-primary", Address: "dns.example.invalid", Role: "dns", Criticality: "critical",
			BackupRequired: true},
	}
	id, err := NewPlanID()
	if err != nil {
		t.Fatalf("NewPlanID: %v", err)
	}
	p := Plan{
		Schema:               PlanSchemaVersion,
		PlanID:               id,
		CreatedAt:            fixedNow.Unix(),
		ExpiresAt:            fixedNow.Add(DefaultPlanTTL).Unix(),
		Environment:          "homelab",
		Policy:               PolicySecurityOnly,
		Hosts:                hosts,
		HostOrder:            OrderHosts(hosts),
		BatchSize:            3,
		RebootPolicy:         RebootPolicyNever,
		SafetyClassification: "disruptive",
		Backup: BackupState{
			RequiredHosts: []string{"pve-primary", "dns-primary"},
			MaxAgeHours:   26,
			Satisfied:     false,
			Detail:        "backup state unknown: no environment linkage",
		},
		Infra: InfraSnapshot{Environment: "homelab", Overall: "healthy", MaintenanceSafe: true},
	}
	final, err := Finalize(p)
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	return final
}

func TestPlanVerifyHappyPath(t *testing.T) {
	p := samplePlan(t)
	if err := Verify(p, fixedNow); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

func TestPlanDigestDeterministic(t *testing.T) {
	p := samplePlan(t)
	d1, err := computeDigest(p)
	if err != nil {
		t.Fatalf("digest: %v", err)
	}
	d2, err := computeDigest(p)
	if err != nil {
		t.Fatalf("digest: %v", err)
	}
	if d1 != d2 {
		t.Error("digest is not deterministic")
	}
}

func TestPlanTamperDetected(t *testing.T) {
	mutations := []struct {
		name   string
		mutate func(*Plan)
	}{
		{"policy change", func(p *Plan) { p.Policy = PolicyApprovedFull }},
		{"host added", func(p *Plan) {
			p.Hosts = append(p.Hosts, PlanHost{Name: "evil", Address: "evil.example.invalid", Role: "generic"})
		}},
		{"host order change", func(p *Plan) { p.HostOrder[0], p.HostOrder[1] = p.HostOrder[1], p.HostOrder[0] }},
		{"reboot policy change", func(p *Plan) { p.RebootPolicy = "always" }},
		{"expiry extended", func(p *Plan) { p.ExpiresAt += 86400 }},
		{"backup requirement dropped", func(p *Plan) { p.Backup.RequiredHosts = nil }},
		{"backup satisfied flipped", func(p *Plan) { p.Backup.Satisfied = true }},
		{"blocker removed", func(p *Plan) { p.Blockers = nil }},
		{"safety weakened", func(p *Plan) { p.SafetyClassification = "observation" }},
	}
	for _, tt := range mutations {
		t.Run(tt.name, func(t *testing.T) {
			p := samplePlan(t)
			p.Blockers = []string{"seed blocker"}
			var err error
			p, err = Finalize(p)
			if err != nil {
				t.Fatalf("Finalize: %v", err)
			}
			tt.mutate(&p)
			if err := Verify(p, fixedNow); err == nil {
				t.Error("tampered plan passed verification")
			}
		})
	}
}

func TestPlanTamperViaJSONRoundTrip(t *testing.T) {
	p := samplePlan(t)
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	tampered := strings.Replace(string(raw), "security-only", "approved-full-upgrade", 1)
	var p2 Plan
	if err := json.Unmarshal([]byte(tampered), &p2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := Verify(p2, fixedNow); err == nil {
		t.Error("JSON-tampered plan passed verification")
	}

	// The untampered round trip must verify.
	var p3 Plan
	if err := json.Unmarshal(raw, &p3); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := Verify(p3, fixedNow); err != nil {
		t.Errorf("clean round-trip failed verification: %v", err)
	}
}

func TestPlanExpiry(t *testing.T) {
	p := samplePlan(t)
	if err := Verify(p, fixedNow.Add(DefaultPlanTTL+time.Minute)); err == nil {
		t.Error("expired plan passed verification")
	}
	if err := Verify(p, fixedNow.Add(DefaultPlanTTL-time.Minute)); err != nil {
		t.Errorf("unexpired plan failed: %v", err)
	}
}

func TestPlanRejectsUnknownPolicyAndSchema(t *testing.T) {
	p := samplePlan(t)
	p.Policy = "yolo-upgrade"
	p, _ = Finalize(p)
	if err := Verify(p, fixedNow); err == nil {
		t.Error("unknown policy passed verification")
	}

	p2 := samplePlan(t)
	p2.Schema = 99
	p2, _ = Finalize(p2)
	if err := Verify(p2, fixedNow); err == nil {
		t.Error("unknown schema passed verification")
	}
}

func TestPlanContainsNoSecretLikeContent(t *testing.T) {
	p := samplePlan(t)
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	lower := strings.ToLower(string(raw))
	for _, forbidden := range []string{"password", "token_secret", "private_key", "authorization", "cookie", "vault"} {
		if strings.Contains(lower, forbidden) {
			t.Errorf("serialized plan contains %q", forbidden)
		}
	}
}

func TestOrderHostsInfraLast(t *testing.T) {
	p := samplePlan(t)
	order := p.HostOrder
	if len(order) != 3 {
		t.Fatalf("order = %v", order)
	}
	if order[0] != "web1" {
		t.Errorf("standard host must come first: %v", order)
	}
	// pve/pbs/dns roles come last, alphabetically among themselves.
	if order[1] != "dns-primary" || order[2] != "pve-primary" {
		t.Errorf("infrastructure roles must come last: %v", order)
	}
}

func TestPlanIDUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		id, err := NewPlanID()
		if err != nil {
			t.Fatalf("NewPlanID: %v", err)
		}
		if seen[id] {
			t.Fatalf("duplicate plan id %s", id)
		}
		seen[id] = true
		if !strings.HasPrefix(id, "mp-") {
			t.Errorf("id %q missing prefix", id)
		}
	}
}

func TestInterpretCheckUpdates(t *testing.T) {
	res := &ansible.RunResult{
		Hosts: []ansible.HostResult{
			{Host: "web1"},
			{Host: "down1", Unreachable: 1, Failed: true},
		},
		TaskOutcomes: map[string][]ansible.TaskOutcome{
			"web1": {
				{Task: "List upgradable packages", StdoutLines: []string{
					"Listing... Done",
					"nano/stable 8.0-1 amd64 [upgradable from: 7.2-1]",
					"openssl/stable-security 3.0.15-1 amd64 [upgradable from: 3.0.14-1]",
				}},
				{Task: "Simulate dist-upgrade", StdoutLines: []string{
					"Inst openssl [3.0.14-1] (3.0.15-1 Debian-Security:12/stable-security [amd64])",
					"Inst nano [7.2-1] (8.0-1 Debian:12.6/stable [amd64])",
					"Conf openssl (3.0.15-1 Debian-Security:12/stable-security [amd64])",
				}},
				{Task: "Check reboot-required marker", StatExists: boolPtr(true)},
				{Task: "List failed systemd units", StdoutLines: []string{"smartd.service loaded failed failed Self Monitoring"}},
				{Task: "Report root filesystem usage", StdoutLines: []string{
					"Filesystem 1024-blocks Used Available Capacity Mounted on",
					"/dev/sda1 41152736 12345678 27000000 32% /",
				}},
			},
		},
	}
	statuses := InterpretCheckUpdates(res)
	if len(statuses) != 2 {
		t.Fatalf("expected 2 statuses, got %d", len(statuses))
	}
	web := statuses[0]
	if web.Host != "web1" || !web.Reachable || !web.Supported {
		t.Errorf("web1 status wrong: %+v", web)
	}
	if len(web.PendingUpdates) != 2 || web.PendingUpdates[0] != "nano" || web.PendingUpdates[1] != "openssl" {
		t.Errorf("pending updates wrong: %v", web.PendingUpdates)
	}
	if len(web.SecurityUpdates) != 1 || web.SecurityUpdates[0] != "openssl" {
		t.Errorf("security updates wrong: %v", web.SecurityUpdates)
	}
	if !web.RebootRequired {
		t.Error("reboot-required not detected")
	}
	if len(web.FailedUnits) != 1 || web.FailedUnits[0] != "smartd.service" {
		t.Errorf("failed units wrong: %v", web.FailedUnits)
	}
	if web.RootUsage != "32%" {
		t.Errorf("root usage = %q", web.RootUsage)
	}

	down := statuses[1]
	if down.Reachable || down.Supported {
		t.Errorf("unreachable host misreported: %+v", down)
	}
}

func TestInterpretUnsupportedDistribution(t *testing.T) {
	res := &ansible.RunResult{
		Hosts: []ansible.HostResult{{Host: "bsd1", Failures: 1, Failed: true}},
		TaskOutcomes: map[string][]ansible.TaskOutcome{
			"bsd1": {{Task: "Verify Debian family", Failed: true, Message: "unsupported distribution"}},
		},
	}
	statuses := InterpretCheckUpdates(res)
	if statuses[0].Supported {
		t.Error("non-Debian host must be unsupported")
	}
}

func boolPtr(b bool) *bool { return &b }
