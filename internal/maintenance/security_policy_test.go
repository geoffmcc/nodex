package maintenance

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildSecurityPolicyConfigClearsNonSecurityOriginsAndNeverReboots(t *testing.T) {
	config, err := BuildSecurityPolicyConfig(
		[]string{"o=Debian,a=bookworm-security,l=Debian-Security"},
		[]string{"origin=UbuntuESMApps,codename=jammy-apps-security"},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		SecurityPolicyMarker,
		"#clear Unattended-Upgrade::Allowed-Origins;",
		"#clear Unattended-Upgrade::Origins-Pattern;",
		"a=bookworm-security",
		"jammy-apps-security",
		`APT::Periodic::Unattended-Upgrade "1";`,
		`Unattended-Upgrade::Automatic-Reboot "false";`,
	} {
		if !strings.Contains(config, want) {
			t.Errorf("generated policy omitted %q:\n%s", want, config)
		}
	}
	if strings.Contains(config, "bookworm-updates") || strings.Contains(config, "jammy-updates") {
		t.Fatalf("non-security origin was included:\n%s", config)
	}
}

func TestBuildSecurityPolicyConfigRequiresSecurityOrigins(t *testing.T) {
	if _, err := BuildSecurityPolicyConfig([]string{"o=Debian,a=bookworm-updates"}, nil); err == nil {
		t.Fatal("non-security origins should not authorize unattended upgrades")
	}
	if _, err := BuildSecurityPolicyConfig([]string{"o=Example,a=nonsecurity"}, nil); err == nil {
		t.Fatal("a non-security origin containing the substring security should not be accepted")
	}
	if _, err := BuildSecurityPolicyConfig(nil, nil); err == nil {
		t.Fatal("empty origin lists should not authorize unattended upgrades")
	}
}

func TestSecurityPolicyConfigParsesWithAPTConfigWhenAvailable(t *testing.T) {
	aptConfig, err := exec.LookPath("apt-config")
	if err != nil {
		t.Skip("apt-config is not installed on this platform")
	}
	content, err := BuildSecurityPolicyConfig(
		[]string{"o=Debian,a=bookworm-security,l=Debian-Security"},
		[]string{"origin=Debian,codename=${distro_codename}-security,label=Debian-Security"},
	)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "99nodex-unattended-security")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(aptConfig, "-c", path, "dump").CombinedOutput() // #nosec G204 -- apt-config was resolved from PATH solely for this optional local syntax test.
	if err != nil {
		t.Fatalf("apt-config rejected generated policy: %v\n%s", err, output)
	}
	for _, want := range []string{"bookworm-security", "Automatic-Reboot \"false\""} {
		if !strings.Contains(string(output), want) {
			t.Errorf("apt-config dump did not contain %q:\n%s", want, output)
		}
	}
}

func TestSecurityPolicyPlanDigestAndExpiry(t *testing.T) {
	config, err := BuildSecurityPolicyConfig([]string{"o=Debian,a=bookworm-security"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0)
	p := SecurityPolicyPlan{
		PlanID:    "msp-test",
		CreatedAt: now.Unix(), ExpiresAt: now.Add(time.Hour).Unix(),
		Hosts: []SecurityPolicyHost{{
			Name: "web1", Address: "web1.example.test", Role: "generic", Criticality: "standard",
			SSHUser: "automation", Distribution: "Debian",
			SecurityAllowedOrigins: []string{"o=Debian,a=bookworm-security"},
			BeforeTimerEnabled:     "disabled", BeforeTimerActive: "inactive",
			AfterConfig: config, AfterConfigChecksum: hashPolicyConfig(config),
		}},
	}
	if err := p.Finalize(); err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if err := p.Verify(now); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	tampered := p
	tampered.Hosts = append([]SecurityPolicyHost(nil), p.Hosts...)
	tampered.Hosts[0].AfterConfig += "# extra\n"
	if err := tampered.Verify(now); err == nil {
		t.Fatal("tampered plan passed verification")
	}
	if err := p.Verify(time.Unix(p.ExpiresAt, 0)); err == nil {
		t.Fatal("expired plan passed verification")
	}
}

func TestSecurityPolicyReceiptPathIsBounded(t *testing.T) {
	if got := SecurityPolicyReceiptPath("/tmp/receipts", "msp-abc123", "apply"); got != "/tmp/receipts/msp-abc123-apply.json" {
		t.Fatalf("receipt path = %q", got)
	}
	if got := SecurityPolicyReceiptPath("/tmp/receipts", "../outside", "restore"); got != "" {
		t.Fatalf("unsafe receipt path = %q", got)
	}
}
