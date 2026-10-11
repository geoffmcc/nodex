package maintenance

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/geoffmcc/nodex/internal/ansible"
)

func TestAPTApprovedVersionDoesNotFollowCandidateDrift(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("isolated signed APT fixture requires Linux apt tools")
	}
	for _, tool := range []string{"apt-get", "apt-cache", "dpkg-deb", "dpkg-scanpackages", "gpg"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("controlled APT fixture requires %s", tool)
		}
	}
	root := t.TempDir()
	architecture, err := runFixtureCommand(root, "dpkg", "--print-architecture")
	if err != nil {
		t.Fatalf("detect APT fixture architecture: %v\n%s", err, architecture)
	}
	architecture = strings.TrimSpace(architecture)

	keyHome := filepath.Join(root, "gnupg")
	if err := os.Mkdir(keyHome, 0o700); err != nil {
		t.Fatal(err)
	}
	keyEnv := append(os.Environ(), "GNUPGHOME="+keyHome)
	generate := exec.Command("gpg", "--batch", "--passphrase", "", "--quick-generate-key", "Nodex disposable APT fixture", "rsa2048", "sign", "0")
	generate.Env = keyEnv
	if output, err := generate.CombinedOutput(); err != nil {
		t.Fatalf("generate disposable repository signing key: %v\n%s", err, output)
	}
	listing := exec.Command("gpg", "--batch", "--with-colons", "--list-keys")
	listing.Env = keyEnv
	keyListing, err := listing.Output()
	if err != nil {
		t.Fatalf("list disposable signing key: %v", err)
	}
	fingerprint := ""
	for _, line := range strings.Split(string(keyListing), "\n") {
		fields := strings.Split(line, ":")
		if len(fields) > 9 && fields[0] == "fpr" {
			fingerprint = fields[9]
			break
		}
	}
	if fingerprint == "" {
		t.Fatal("GnuPG returned no signing-key fingerprint")
	}
	export := exec.Command("gpg", "--batch", "--export", fingerprint)
	export.Env = keyEnv
	keyBytes, err := export.Output()
	if err != nil {
		t.Fatalf("export fixture verification key: %v", err)
	}
	keyring := filepath.Join(root, "fixture-keyring.gpg")
	if err := os.WriteFile(keyring, keyBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	debV2 := buildFixtureDeb(t, root, architecture, "2.0-1")
	debV3 := buildFixtureDeb(t, root, architecture, "3.0-1")
	repo := filepath.Join(root, "repo")
	pool := filepath.Join(repo, "pool", "main", "n", "nodex-fixture")
	indexDir := filepath.Join(repo, "dists", "nodex-security", "main", "binary-"+architecture)
	for _, dir := range []string{pool, indexDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := publishFixtureRepository(t, repo, pool, indexDir, []string{debV2}, keyHome, fingerprint, "NodexFixture", "NodexFixture"); err != nil {
		t.Fatal(err)
	}
	repoURL := (&url.URL{Scheme: "file", Path: repo}).String()
	sourcePath := filepath.Join(root, "sources.list")
	source := fmt.Sprintf("deb [arch=%s signed-by=%s] %s nodex-security main\n", architecture, keyring, repoURL)
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"lists/partial", "archives/partial", "cache"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, dir := range []string{"sourceparts", "trustedparts"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	statusPath := filepath.Join(root, "status")
	status := fmt.Sprintf("Package: nodex-fixture\nStatus: install ok installed\nPriority: optional\nSection: misc\nMaintainer: Nodex Fixture <fixture@example.invalid>\nArchitecture: %s\nVersion: 1.0-1\nDescription: isolated candidate drift fixture\n\n", architecture)
	if err := os.WriteFile(statusPath, []byte(status), 0o644); err != nil {
		t.Fatal(err)
	}
	aptConfig := []string{
		"Dir::Etc::sourcelist=" + sourcePath,
		"Dir::Etc::sourceparts=" + filepath.Join(root, "sourceparts"),
		"Dir::Etc::main=/dev/null",
		"Dir::Etc::parts=-",
		"Dir::Etc::trusted=/dev/null",
		"Dir::Etc::trustedparts=" + filepath.Join(root, "trustedparts"),
		"Dir::State::lists=" + filepath.Join(root, "lists"),
		"Dir::State::status=" + statusPath,
		"Dir::Cache::archives=" + filepath.Join(root, "archives"),
		"Dir::Cache::pkgcache=" + filepath.Join(root, "cache", "pkgcache.bin"),
		"Dir::Cache::srcpkgcache=" + filepath.Join(root, "cache", "srcpkgcache.bin"),
		"Acquire::Languages=none",
		"APT::Get::List-Cleanup=0",
		"Debug::NoLocking=1",
	}
	apt := func(args ...string) (string, error) {
		full := make([]string, 0, 2*len(aptConfig)+len(args))
		for _, option := range aptConfig {
			full = append(full, "-o", option)
		}
		full = append(full, args...)
		return runFixtureCommand(root, "apt-get", full...)
	}
	aptOutput, err := apt("update", "-o", "Acquire::Retries=0")
	if err != nil {
		t.Fatalf("signed local apt-get update: %v\n%s", err, aptOutput)
	}
	if strings.Contains(strings.ToLower(aptOutput), "not signed") {
		t.Fatalf("fixture repository signature was not accepted: %s", aptOutput)
	}
	targets, err := apt("indextargets", "--format", "$(SITE)|$(RELEASE)|$(CODENAME)|$(ORIGIN)|$(LABEL)|$(COMPONENT)|$(ARCHITECTURE)")
	if err != nil {
		t.Fatalf("read APT source metadata: %v\n%s", err, targets)
	}
	evidence, err := runCheckUpdatesAPTFixture(t, root, aptConfig)
	if err != nil {
		t.Fatalf("run embedded check-updates playbook against signed APT fixture: %v", err)
	}
	intents, err := parsePackageIntents(evidence.Simulation, evidence.Policies, evidence.IndexTargets)
	if err != nil || len(intents) != 1 || intents[0].Selector() != "nodex-fixture:"+architecture+"=2.0-1" || len(intents[0].Origins) != 1 || intents[0].Origins[0].Origin != "NodexFixture" {
		t.Fatalf("parsed check-updates APT intent = %+v err=%v; evidence=%+v", intents, err, evidence)
	}
	approved := intents[0]

	if err := publishFixtureRepository(t, repo, pool, indexDir, []string{debV2}, keyHome, fingerprint, "NodexMirror", "NodexMirror"); err != nil {
		t.Fatal(err)
	}
	// APT normally rejects this Release identity mutation itself. Permit it
	// only inside the disposable fixture so the Nodex revalidation comparison
	// is exercised against the newly signed metadata.
	if _, err := apt("update", "--allow-releaseinfo-change", "-o", "Acquire::Retries=0"); err != nil {
		t.Fatalf("refresh metadata after signed origin drift: %v", err)
	}
	targets, err = apt("indextargets", "--format", "$(SITE)|$(RELEASE)|$(CODENAME)|$(ORIGIN)|$(LABEL)|$(COMPONENT)|$(ARCHITECTURE)")
	if err != nil {
		t.Fatalf("read changed APT origin: %v\n%s", err, targets)
	}
	if strings.Contains(targets, approved.Origins[0].Record()) || !strings.Contains(targets, "NodexMirror") {
		t.Fatalf("Release origin drift was not distinguishable from approval: approved=%s current=%s", approved.Origins[0].Record(), targets)
	}
	originDriftErr := runSecurityUpdatePreflightFixture(t, root, aptConfig, packageIntentToApproval(approved))
	if originDriftErr == nil || !strings.Contains(originDriftErr.Error(), "approved APT Release origin") || strings.Contains(originDriftErr.Error(), "reached package mutation") {
		t.Fatalf("embedded Ansible operation did not stop on signed origin drift before package mutation: %v", originDriftErr)
	}

	// The reviewed schema-1 workflow passed names with state=latest. Show the
	// controlled drift explicitly: after the source publishes 3.0 and metadata
	// refreshes, an unpinned install resolves 3.0 while the approved exact 2.0
	// selector refuses because that version has disappeared.
	if err := publishFixtureRepository(t, repo, pool, indexDir, []string{debV3}, keyHome, fingerprint, "NodexMirror", "NodexMirror"); err != nil {
		t.Fatal(err)
	}
	aptOutput, err = apt("update", "-o", "Acquire::Retries=0")
	if err != nil {
		t.Fatalf("refresh metadata after candidate drift: %v\n%s", err, aptOutput)
	}
	approvedMirror := packageIntentToApproval(approved)
	approvedMirror.Origins[0].Origin = "NodexMirror"
	approvedMirror.Origins[0].Label = "NodexMirror"
	approvedMirror.Origins[0].Record = "" // Recompute from the current, separately approved Release identity.
	approvedMirror.Origins[0].Record = packageOriginRecord(approvedMirror.Origins[0])
	candidateDriftErr := runSecurityUpdatePreflightFixture(t, root, aptConfig, approvedMirror)
	if candidateDriftErr == nil || !strings.Contains(candidateDriftErr.Error(), "candidate version, architecture, or APT origin changed") || strings.Contains(candidateDriftErr.Error(), "reached package mutation") {
		t.Fatalf("embedded Ansible operation did not stop on candidate drift before package mutation: %v", candidateDriftErr)
	}
	unpinned, err := apt("-s", "install", "nodex-fixture")
	if err != nil {
		t.Fatalf("simulate prior unpinned latest-style intent: %v\n%s", err, unpinned)
	}
	if !strings.Contains(unpinned, "3.0-1") {
		t.Fatalf("unpinned package request did not follow the changed candidate:\n%s", unpinned)
	}
	pinned, err := apt("-s", "--no-remove", "install", approved.Selector())
	if err == nil || (!strings.Contains(strings.ToLower(pinned), "version") && !strings.Contains(strings.ToLower(pinned), "unable to locate")) {
		t.Fatalf("pinned approved version did not fail closed after drift: err=%v\n%s", err, pinned)
	}
	approvedCurrent := approvedMirror
	approvedCurrent.Version = "3.0-1"
	approvedCurrent.Spec = "nodex-fixture:" + architecture + "=3.0-1"
	mutationSentinel := runSecurityUpdatePreflightFixture(t, root, aptConfig, approvedCurrent)
	if mutationSentinel == nil || !strings.Contains(mutationSentinel.Error(), "reached package mutation after failed preflight") {
		t.Fatalf("embedded Ansible preflight did not accept the matching exact intent before the non-mutating install sentinel: %v", mutationSentinel)
	}
}

func packageIntentToApproval(intent PackageIntent) ansible.PackageApproval {
	origins := make([]ansible.PackageOrigin, 0, len(intent.Origins))
	for _, origin := range intent.Origins {
		origins = append(origins, ansible.PackageOrigin{
			Source: origin.Source, Site: origin.Site, Release: origin.Release, Codename: origin.Codename,
			Origin: origin.Origin, Label: origin.Label, Component: origin.Component,
			Architecture: origin.Architecture, Record: origin.Record(),
		})
	}
	return ansible.PackageApproval{Name: intent.Name, Version: intent.Version, Architecture: intent.Architecture, Origins: origins, Spec: intent.Selector()}
}

type checkUpdatesPackageEvidence struct {
	Simulation   []string            `json:"simulation"`
	Policies     []aptPolicyEvidence `json:"policies"`
	IndexTargets []string            `json:"index_targets"`
}

func runCheckUpdatesAPTFixture(t *testing.T, root string, aptConfig []string) (checkUpdatesPackageEvidence, error) {
	t.Helper()
	executable, err := exec.LookPath("ansible-playbook")
	if err != nil {
		t.Skip("Ansible is required to exercise the embedded package preflight")
	}
	configPath := filepath.Join(root, "check-updates-apt.conf")
	if err := writeFixtureAPTConfig(configPath, aptConfig); err != nil {
		return checkUpdatesPackageEvidence{}, err
	}
	operation, err := ansible.Lookup("check-updates")
	if err != nil {
		return checkUpdatesPackageEvidence{}, err
	}
	playbook := strings.ReplaceAll(operation.Playbook(), "become: true", "become: false")
	playbook = strings.Replace(playbook, "  gather_facts: true\n", "  gather_facts: true\n  environment:\n    APT_CONFIG: \"{{ nodex_test_apt_config }}\"\n", 1)
	playbook = strings.Replace(playbook,
		"      ansible.builtin.apt:\n        update_cache: true",
		"      ansible.builtin.debug:\n        msg: fixture APT metadata was refreshed before this isolated run", 1)
	playbookPath := filepath.Join(root, "check-updates-fixture.yml")
	if err := os.WriteFile(playbookPath, []byte(playbook), 0o600); err != nil {
		return checkUpdatesPackageEvidence{}, err
	}
	inventory := filepath.Join(root, "check-updates-inventory.ini")
	if err := os.WriteFile(inventory, []byte("[all]\nfixture ansible_connection=local\n"), 0o600); err != nil {
		return checkUpdatesPackageEvidence{}, err
	}
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		return checkUpdatesPackageEvidence{}, fmt.Errorf("locate embedded Ansible callback fixture")
	}
	callbackDir := filepath.Clean(filepath.Join(filepath.Dir(source), "..", "ansible", "callback"))
	ansibleConfig := filepath.Join(root, "check-updates-ansible.cfg")
	configContents := fmt.Sprintf("[defaults]\nstdout_callback = nodex_json\ncallback_plugins = %s\nhost_key_checking = True\nretry_files_enabled = False\n", callbackDir)
	if err := os.WriteFile(ansibleConfig, []byte(configContents), 0o600); err != nil {
		return checkUpdatesPackageEvidence{}, err
	}
	vars, err := json.Marshal(map[string]string{"nodex_test_apt_config": configPath})
	if err != nil {
		return checkUpdatesPackageEvidence{}, err
	}
	localTmp := filepath.Join(root, "check-updates-local-tmp")
	if err := os.MkdirAll(localTmp, 0o700); err != nil {
		return checkUpdatesPackageEvidence{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, "-i", inventory, "--extra-vars", string(vars), playbookPath) // #nosec G204 -- executable/playbook are test fixtures; args are generated paths
	command.Env = append(os.Environ(), "ANSIBLE_CONFIG="+ansibleConfig, "ANSIBLE_LOCAL_TEMP="+localTmp, "PYTHONDONTWRITEBYTECODE=1")
	output, err := command.CombinedOutput()
	if err != nil {
		return checkUpdatesPackageEvidence{}, fmt.Errorf("embedded check-updates playbook: %w\n%s", err, output)
	}
	var callback struct {
		Plays []struct {
			Tasks []struct {
				Task struct {
					EvidenceID string `json:"evidence_id"`
				} `json:"task"`
				Hosts map[string]struct {
					Message string `json:"msg"`
				} `json:"hosts"`
			} `json:"tasks"`
		} `json:"plays"`
	}
	if err := json.Unmarshal(output, &callback); err != nil {
		return checkUpdatesPackageEvidence{}, fmt.Errorf("parse embedded check-updates callback: %w", err)
	}
	for _, play := range callback.Plays {
		for _, task := range play.Tasks {
			if task.Task.EvidenceID != ansible.HostEvidencePackagePolicy {
				continue
			}
			var evidence checkUpdatesPackageEvidence
			if err := json.Unmarshal([]byte(task.Hosts["fixture"].Message), &evidence); err != nil {
				return checkUpdatesPackageEvidence{}, fmt.Errorf("decode check-updates package evidence: %w", err)
			}
			return evidence, nil
		}
	}
	return checkUpdatesPackageEvidence{}, fmt.Errorf("embedded check-updates output lacks package-policy evidence")
}

func packageOriginRecord(origin ansible.PackageOrigin) string {
	return strings.Join([]string{origin.Site, origin.Release, origin.Codename, origin.Origin, origin.Label, origin.Component, origin.Architecture}, "|")
}

func runSecurityUpdatePreflightFixture(t *testing.T, root string, aptConfig []string, approved ansible.PackageApproval) error {
	t.Helper()
	executable, err := exec.LookPath("ansible-playbook")
	if err != nil {
		t.Skip("Ansible is required for the APT assertion integration")
	}
	configFile := filepath.Join(root, "apt-fixture.conf")
	if err := writeFixtureAPTConfig(configFile, aptConfig); err != nil {
		return err
	}
	operation, err := ansible.Lookup("apply-security-updates")
	if err != nil {
		return err
	}
	playbook := strings.ReplaceAll(operation.Playbook(), "become: true", "become: false")
	playbook = strings.Replace(playbook, "  gather_facts: true\n", "  gather_facts: true\n  environment:\n    APT_CONFIG: \"{{ nodex_test_apt_config }}\"\n", 1)
	playbook = strings.Replace(playbook,
		"      ansible.builtin.apt:\n        update_cache: true\n",
		"      ansible.builtin.debug:\n        msg: fixture metadata was refreshed immediately before this isolated run\n", 1)
	playbook = strings.Replace(playbook,
		"      ansible.builtin.apt:\n        name: \"{{ nodex_packages | map(attribute='spec') | list }}\"\n        state: present\n        only_upgrade: true\n        fail_on_autoremove: true\n        install_recommends: false\n        allow_unauthenticated: false\n",
		"      ansible.builtin.fail:\n        msg: reached package mutation after failed preflight\n", 1)
	if strings.Contains(playbook, "ansible.builtin.apt:") || !strings.Contains(playbook, "reached package mutation after failed preflight") {
		return fmt.Errorf("isolated Ansible test failed to guard package mutation")
	}
	playbookPath := filepath.Join(root, "security-update-preflight.yml")
	if err := os.WriteFile(playbookPath, []byte(playbook), 0o600); err != nil {
		return err
	}
	inventory := filepath.Join(root, "security-update-inventory.ini")
	if err := os.WriteFile(inventory, []byte("[all]\nfixture ansible_connection=local\n"), 0o600); err != nil {
		return err
	}
	ansibleConfig := filepath.Join(root, "security-update-ansible.cfg")
	if err := os.WriteFile(ansibleConfig, []byte("[defaults]\nhost_key_checking = True\nretry_files_enabled = False\n"), 0o600); err != nil {
		return err
	}
	vars, err := json.Marshal(map[string]any{
		"nodex_packages":        []ansible.PackageApproval{approved},
		"nodex_test_apt_config": configFile,
	})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, "-i", inventory, "--extra-vars", string(vars), playbookPath) // #nosec G204 -- executable/playbook are test fixtures; args are generated paths
	command.Env = append(os.Environ(), "ANSIBLE_CONFIG="+ansibleConfig, "ANSIBLE_LOCAL_TEMP="+filepath.Join(root, "ansible-local-tmp"))
	if err := os.MkdirAll(filepath.Join(root, "ansible-local-tmp"), 0o700); err != nil {
		return err
	}
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ansible preflight: %w\n%s", err, output)
	}
	return nil
}

func writeFixtureAPTConfig(path string, settings []string) error {
	var contents strings.Builder
	for _, setting := range settings {
		key, value, ok := strings.Cut(setting, "=")
		if !ok {
			return fmt.Errorf("invalid fixture APT setting")
		}
		fmt.Fprintf(&contents, "%s \"%s\";\n", key, value)
	}
	return os.WriteFile(path, []byte(contents.String()), 0o600)
}

func buildFixtureDeb(t *testing.T, root, architecture, version string) string {
	t.Helper()
	dir := filepath.Join(root, "build-"+version)
	if err := os.MkdirAll(filepath.Join(dir, "DEBIAN"), 0o755); err != nil {
		t.Fatal(err)
	}
	control := fmt.Sprintf("Package: nodex-fixture\nVersion: %s\nArchitecture: %s\nMaintainer: Nodex Fixture <fixture@example.invalid>\nDescription: disposable test package\n", version, architecture)
	if err := os.WriteFile(filepath.Join(dir, "DEBIAN", "control"), []byte(control), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "usr", "share", "nodex-fixture"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "usr", "share", "nodex-fixture", "version"), []byte(version+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	deb := filepath.Join(root, "nodex-fixture_"+version+"_"+architecture+".deb")
	output, err := runFixtureCommand(root, "dpkg-deb", "--build", dir, deb)
	if err != nil {
		t.Fatalf("build local fixture .deb: %v\n%s", err, output)
	}
	return deb
}

func publishFixtureRepository(t *testing.T, repo, pool, indexDir string, debs []string, keyHome, fingerprint, origin, label string) error {
	t.Helper()
	for _, name := range []string{"nodex-fixture_2.0-1_amd64.deb", "nodex-fixture_3.0-1_amd64.deb"} {
		_ = os.Remove(filepath.Join(pool, name))
	}
	for _, deb := range debs {
		data, err := os.ReadFile(deb)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(pool, filepath.Base(deb)), data, 0o644); err != nil { // #nosec G703 -- deb is a fixture path under the test's private temporary directory
			return err
		}
	}
	scan := exec.Command("dpkg-scanpackages", "--multiversion", "pool", "/dev/null")
	scan.Dir = repo
	plain, err := scan.Output()
	if err != nil {
		return fmt.Errorf("scan fixture packages: %w", err)
	}
	if err := os.WriteFile(filepath.Join(indexDir, "Packages"), plain, 0o644); err != nil {
		return err
	}
	var compressed bytes.Buffer
	w := gzip.NewWriter(&compressed)
	if _, err := w.Write(plain); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	gz := compressed.Bytes()
	if err := os.WriteFile(filepath.Join(indexDir, "Packages.gz"), gz, 0o644); err != nil {
		return err
	}
	plainHash := sha256.Sum256(plain)
	gzHash := sha256.Sum256(gz)
	release := fmt.Sprintf("Origin: %s\nLabel: %s\nSuite: nodex-security\nCodename: nodex-security\nDate: %s\nArchitectures: amd64\nComponents: main\nDescription: disposable Nodex package fixture\nSHA256:\n %s %d main/binary-amd64/Packages\n %s %d main/binary-amd64/Packages.gz\n", origin, label, time.Now().UTC().Format(time.RFC1123Z), hex.EncodeToString(plainHash[:]), len(plain), hex.EncodeToString(gzHash[:]), len(gz))
	releasePath := filepath.Join(repo, "dists", "nodex-security", "Release")
	if err := os.WriteFile(releasePath, []byte(release), 0o644); err != nil {
		return err
	}
	command := exec.Command("gpg", "--batch", "--yes", "--homedir", keyHome, "--local-user", fingerprint, "--output", filepath.Join(repo, "dists", "nodex-security", "InRelease"), "--clearsign", releasePath) // #nosec G204 -- gpg is a fixed system tool; arguments are fixture-scoped paths
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("sign Release metadata: %w: %s", err, output)
	}
	return nil
}

func runFixtureCommand(dir, binary string, args ...string) (string, error) {
	command := exec.Command(binary, args...) // #nosec G204 -- binary and args are test-controlled fixture helpers
	command.Dir = dir
	output, err := command.CombinedOutput()
	return string(output), err
}
