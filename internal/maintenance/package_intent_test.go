package maintenance

import (
	"strings"
	"testing"
)

func TestParsePackageIntentsBindsVersionArchitectureAndOrigin(t *testing.T) {
	intents, err := parsePackageIntents(
		[]string{
			"Inst openssl [3.0.14-1] (3.0.15-1 Debian-Security:12/stable-security [amd64])",
			"Conf openssl (3.0.15-1 Debian-Security:12/stable-security [amd64])",
		},
		[]aptPolicyEvidence{{
			Item: "openssl", RC: 0,
			StdoutLines: []string{
				"openssl:", "  Candidate: 3.0.15-1", "  Version table:", " *** 3.0.15-1 500",
				"        500 https://security.debian.org/debian-security stable-security/main amd64 Packages",
				"        100 /var/lib/dpkg/status",
			},
		}},
		[]string{"https://security.debian.org/debian-security|stable-security|bookworm-security|Debian|Debian-Security|main|amd64"},
	)
	if err != nil {
		t.Fatalf("parsePackageIntents: %v", err)
	}
	if len(intents) != 1 || intents[0].Selector() != "openssl:amd64=3.0.15-1" {
		t.Fatalf("package intent = %+v", intents)
	}
	wantOrigin := APTOrigin{
		Source: "https://security.debian.org/debian-security stable-security/main amd64", Site: "https://security.debian.org/debian-security",
		Release: "stable-security", Codename: "bookworm-security", Origin: "Debian", Label: "Debian-Security", Component: "main", Architecture: "amd64",
	}
	if len(intents[0].Origins) != 1 || intents[0].Origins[0] != wantOrigin {
		t.Fatalf("origins = %v, want %q", intents[0].Origins, wantOrigin)
	}
}

func TestParsePackageIntentsHandlesAPTMultiOriginAndResolverAnnotations(t *testing.T) {
	simulation := []string{
		"Inst bsdutils [1:2.41-5] (1:2.41.5-0+deb13u1 Debian:13.7/stable, Debian-Security:13/stable-security [amd64]) [util-linux:amd64 on liblastlog2-2:amd64]",
	}
	intents, err := parsePackageIntents(
		simulation,
		[]aptPolicyEvidence{{
			Item: "bsdutils", RC: 0,
			StdoutLines: []string{
				"bsdutils:", "  Candidate: 1:2.41.5-0+deb13u1", "  Version table:",
				" *** 1:2.41.5-0+deb13u1 500",
				"        500 http://deb.debian.org/debian trixie/main amd64 Packages",
				"        500 http://security.debian.org/debian-security trixie-security/main amd64 Packages",
				"        100 /var/lib/dpkg/status",
			},
		}},
		[]string{
			"http://deb.debian.org/debian|trixie|trixie|Debian|Debian|main|amd64",
			"http://security.debian.org/debian-security|trixie-security|trixie-security|Debian|Debian-Security|main|amd64",
		},
	)
	if err != nil {
		t.Fatalf("parse multi-origin package intent: %v", err)
	}
	if len(intents) != 1 || intents[0].Selector() != "bsdutils:amd64=1:2.41.5-0+deb13u1" {
		t.Fatalf("package intent = %+v", intents)
	}
	wantOrigin := APTOrigin{
		Source: "http://security.debian.org/debian-security trixie-security/main amd64",
		Site:   "http://security.debian.org/debian-security", Release: "trixie-security", Codename: "trixie-security",
		Origin: "Debian", Label: "Debian-Security", Component: "main", Architecture: "amd64",
	}
	if len(intents[0].Origins) != 1 || intents[0].Origins[0] != wantOrigin {
		t.Fatalf("origins = %+v, want security origin %+v", intents[0].Origins, wantOrigin)
	}
	if updates, err := parseSecurityUpdatesChecked(simulation); err != nil || len(updates) != 1 || updates[0] != "bsdutils" {
		t.Fatalf("security updates = %v, err = %v", updates, err)
	}
}

func TestParsePackageIntentsBindsAllArchitecturePackageToNativeIndex(t *testing.T) {
	intents, err := parsePackageIntents(
		[]string{"Inst python3-urllib3 [2.3.0-3+deb13u1] (2.3.0-3+deb13u2 Debian:13.7/stable, Debian-Security:13/stable-security [all])"},
		[]aptPolicyEvidence{{
			Item: "python3-urllib3", RC: 0,
			StdoutLines: []string{
				"python3-urllib3:", "  Candidate: 2.3.0-3+deb13u2", "  Version table:",
				" *** 2.3.0-3+deb13u2 500",
				"        500 http://deb.debian.org/debian trixie/main amd64 Packages",
				"        500 http://security.debian.org/debian-security trixie-security/main amd64 Packages",
				"        100 /var/lib/dpkg/status",
			},
		}},
		[]string{
			"http://deb.debian.org/debian|trixie|trixie|Debian|Debian|main|amd64",
			"http://security.debian.org/debian-security|trixie-security|trixie-security|Debian|Debian-Security|main|amd64",
		},
	)
	if err != nil {
		t.Fatalf("parse architecture-independent package intent: %v", err)
	}
	if len(intents) != 1 || intents[0].Selector() != "python3-urllib3:all=2.3.0-3+deb13u2" {
		t.Fatalf("package intent = %+v", intents)
	}
	if len(intents[0].Origins) != 1 || intents[0].Origins[0].Architecture != "amd64" {
		t.Fatalf("signed index origin = %+v, want native amd64 index", intents[0].Origins)
	}
	if err := validateSecurityPackageIntents([]string{"python3-urllib3"}, intents); err != nil {
		t.Fatalf("validate architecture-independent package intent: %v", err)
	}
	intents[0].Origins[0].Architecture = "arm64"
	if err := validateSecurityPackageIntents([]string{"python3-urllib3"}, intents); err == nil {
		t.Fatal("accepted origin architecture that differs from its signed APT source")
	}
}

func TestParsePackageIntentsRejectsMissingCandidateSource(t *testing.T) {
	_, err := parsePackageIntents(
		[]string{"Inst openssl [3.0.14-1] (3.0.15-1 Debian-Security:12/stable-security [amd64])"},
		[]aptPolicyEvidence{{Item: "openssl", RC: 0, StdoutLines: []string{"Candidate: 3.0.15-1"}}},
		[]string{"https://security.debian.org/debian-security|stable-security|bookworm-security|Debian|Debian-Security|main|amd64"},
	)
	if err == nil || !strings.Contains(err.Error(), "source") {
		t.Fatalf("missing policy origin error = %v", err)
	}
}

func TestParsePackageIntentsRejectsCredentialBearingSourceURI(t *testing.T) {
	_, err := parsePackageIntents(
		[]string{"Inst openssl [3.0.14-1] (3.0.15-1 Debian-Security:12/stable-security [amd64])"},
		[]aptPolicyEvidence{{Item: "openssl", RC: 0, StdoutLines: []string{
			"Candidate: 3.0.15-1", " *** 3.0.15-1 500",
			"  500 https://operator:private-token@repo.invalid/debian stable-security/main amd64 Packages",
		}}},
		[]string{"https://repo.invalid/debian|stable-security|bookworm-security|Debian|Debian-Security|main|amd64"},
	)
	if err == nil || strings.Contains(err.Error(), "private-token") {
		t.Fatalf("credential-bearing APT URI result = %v; want safe refusal without disclosure", err)
	}
}

func TestParsePackageIntentsSupportsMultipleArchitecturesForOnePackage(t *testing.T) {
	intent, err := parsePackageIntents(
		[]string{
			"Inst libfoo:amd64 [1.0-1] (2.0-1 nodex-security [amd64])",
			"Inst libfoo:i386 [1.0-1] (2.0-1 nodex-security [i386])",
		},
		[]aptPolicyEvidence{
			{Item: "libfoo:amd64", StdoutLines: []string{"Candidate: 2.0-1", " *** 2.0-1 500", "  500 https://repo.invalid/debian nodex-security/main amd64 Packages"}},
			{Item: "libfoo:i386", StdoutLines: []string{"Candidate: 2.0-1", " *** 2.0-1 500", "  500 https://repo.invalid/debian nodex-security/main i386 Packages"}},
		},
		[]string{
			"https://repo.invalid/debian|nodex-security|nodex-security|Nodex|Nodex|main|amd64",
			"https://repo.invalid/debian|nodex-security|nodex-security|Nodex|Nodex|main|i386",
		},
	)
	if err != nil {
		t.Fatalf("parse multiarch package intent: %v", err)
	}
	if len(intent) != 2 || intent[0].Name != "libfoo" || intent[1].Name != "libfoo" || intent[0].Architecture == intent[1].Architecture {
		t.Fatalf("multiarch intent = %+v", intent)
	}
	if err := validateSecurityPackageIntents([]string{"libfoo"}, intent); err != nil {
		t.Fatalf("validate multiarch package approval: %v", err)
	}
}

func TestSecurityPackageClassificationUsesAPTArchiveNotPackageName(t *testing.T) {
	got := parseSecurityUpdates([]string{
		"Inst esmtp [1.0-1] (2.0-1 Debian:stable [amd64])",
		"Inst libesm-helper [1.0-1] (2.0-1 Debian:stable [amd64])",
		"Inst openssl-security-addon [1.0-1] (2.0-1 Debian:stable [amd64])",
		"Inst openssl [3.0-1] (3.1-1 Debian-Security:stable-security [amd64])",
	})
	if len(got) != 1 || got[0] != "openssl" {
		t.Fatalf("security packages = %v; package names must not be mistaken for archive origins", got)
	}
}

func TestSecurityUpdateClassificationRejectsMalformedArchiveEvidence(t *testing.T) {
	_, err := parseSecurityUpdatesChecked([]string{"Inst openssl (unparseable-security-entry)"})
	if err == nil {
		t.Fatal("malformed security simulation was treated as no pending security update")
	}
}

func TestPlanRejectsInsufficientOldSecuritySchema(t *testing.T) {
	plan := Plan{Schema: 1, Policy: PolicySecurityOnly}
	err := Verify(plan, fixedNow)
	if err == nil || !strings.Contains(err.Error(), "regenerate the plan") {
		t.Fatalf("old schema error = %v, want actionable regeneration guidance", err)
	}
}

func TestSecurityPackageIntentIsDigestBound(t *testing.T) {
	plan := samplePlan(t)
	for i := range plan.Hosts {
		if plan.Hosts[i].Name == "web1" {
			plan.Hosts[i].SecurityPackages[0].Origins[0].Origin = "Mirror"
			break
		}
	}
	if err := Verify(plan, fixedNow); err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("origin mutation error = %v, want digest mismatch", err)
	}
}

func TestSnapshotComparisonBlocksSignedOriginDrift(t *testing.T) {
	approved := PackageIntent{Name: "openssl", Version: "3.0.15-1", Architecture: "amd64", Origins: []APTOrigin{{
		Source: "https://security.debian.org/debian-security stable-security/main amd64", Site: "https://security.debian.org/debian-security",
		Release: "stable-security", Codename: "bookworm-security", Origin: "Debian", Label: "Debian-Security", Component: "main", Architecture: "amd64",
	}}}
	host := HostSnapshot{Name: "web1", Address: "web1.example.invalid", Role: "generic", Criticality: "standard", Reachable: true, Supported: true, RootUsage: "10%", SecurityUpdates: []string{"openssl"}, SecurityPackages: []PackageIntent{approved}, EvidenceComplete: true}
	planned := SafetySnapshot{Version: 1, Hosts: map[string]HostSnapshot{"web1": host}, Infrastructure: InfraSnapshot{Overall: "healthy", MaintenanceSafe: true, EvidenceComplete: true}, EvidenceComplete: true}
	currentHost := host
	currentHost.SecurityPackages = append([]PackageIntent(nil), host.SecurityPackages...)
	currentHost.SecurityPackages[0].Origins = append([]APTOrigin(nil), host.SecurityPackages[0].Origins...)
	currentHost.SecurityPackages[0].Origins[0].Origin = "NodexMirror"
	current := planned
	current.Hosts = map[string]HostSnapshot{"web1": currentHost}
	comparison := CompareSnapshots(planned, current)
	if comparison.Overall != DispositionBlocked || !strings.Contains(strings.Join(comparison.ChangedFields, ","), "security_package_intent") {
		t.Fatalf("origin drift comparison = %+v, want blocked package intent", comparison)
	}
}
