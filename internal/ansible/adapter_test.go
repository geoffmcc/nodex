package ansible

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestHelperProcess is not a real test: it is the stub ansible-playbook
// executable. The adapter's testArgsPrefix routes execution back into this
// test binary, which keeps the stub cross-platform (no shell scripts).
func TestHelperProcess(t *testing.T) {
	if os.Getenv("NODEX_ANSIBLE_STUB") != "1" {
		return
	}
	defer os.Exit(0)

	mode := os.Getenv("NODEX_ANSIBLE_STUB_MODE")
	hosts := strings.Split(os.Getenv("NODEX_ANSIBLE_STUB_HOSTS"), ",")

	stats := func(entries map[string]map[string]int) string {
		b, _ := json.Marshal(map[string]any{"stats": entries})
		return string(b)
	}
	okStats := map[string]map[string]int{}
	for _, h := range hosts {
		if h != "" {
			okStats[h] = map[string]int{"ok": 5, "changed": 0, "failures": 0, "unreachable": 0, "skipped": 0}
		}
	}

	switch mode {
	case "ok":
		fmt.Println(stats(okStats))
	case "one-failed":
		if len(hosts) > 0 {
			okStats[hosts[0]] = map[string]int{"ok": 2, "changed": 0, "failures": 1, "unreachable": 0, "skipped": 0}
		}
		fmt.Println(stats(okStats))
		os.Exit(2) // ansible exits 2 when hosts failed
	case "unreachable":
		if len(hosts) > 0 {
			okStats[hosts[0]] = map[string]int{"ok": 0, "changed": 0, "failures": 0, "unreachable": 1, "skipped": 0}
		}
		fmt.Println(stats(okStats))
		os.Exit(4)
	case "missing-host":
		delete(okStats, hosts[len(hosts)-1])
		fmt.Println(stats(okStats))
	case "task-detail":
		payload := map[string]any{
			"stats": okStats,
			"plays": []any{map[string]any{
				"tasks": []any{
					map[string]any{
						"task": map[string]any{"name": "List upgradable packages"},
						"hosts": map[string]any{hosts[0]: map[string]any{
							"stdout_lines": []string{"Listing...", "nano/stable 8.0-1 amd64 [upgradable from: 7.2-1]"},
						}},
					},
					map[string]any{
						"task": map[string]any{"name": "Check reboot-required marker"},
						"hosts": map[string]any{hosts[0]: map[string]any{
							"stat": map[string]any{"exists": true},
						}},
					},
					map[string]any{
						"task": map[string]any{"name": "List failed systemd units"},
						"hosts": map[string]any{hosts[0]: map[string]any{
							"stdout_lines": []string{},
						}},
					},
				},
			}},
		}
		b, _ := json.Marshal(payload)
		fmt.Println(string(b))
	case "bad-json":
		fmt.Println("PLAY RECAP *** not json at all")
	case "exit1-good-stats":
		fmt.Println(stats(okStats))
		os.Exit(1)
	case "env-dump":
		b, _ := json.Marshal(os.Environ())
		fmt.Println(string(b))
	case "huge-output":
		line := strings.Repeat("x", 1024)
		for i := 0; i < 4096; i++ {
			fmt.Println(line)
		}
	case "secret-output":
		fmt.Println(`{"stats":{}} PBSAPIToken=root@pbs!leak:stub-secret-value and password: hunter2`)
	case "hang":
		time.Sleep(5 * time.Minute)
	case "inspect-files":
		// Echo the generated inventory and playbook back for assertions.
		var inv, pb string
		args := os.Args
		for i, a := range args {
			if a == "-i" && i+1 < len(args) {
				inv = args[i+1]
			}
			if strings.HasSuffix(a, "playbook.yml") {
				pb = a
			}
		}
		invData, _ := os.ReadFile(inv) // #nosec G304 G703 -- stub reads adapter-generated file
		pbData, _ := os.ReadFile(pb)   // #nosec G304 G703 -- stub reads adapter-generated file
		payload := map[string]any{
			"stats":     map[string]any{},
			"inventory": string(invData),
			"playbook":  string(pbData),
			"cwd":       mustGetwd(),
		}
		b, _ := json.Marshal(payload)
		fmt.Println(string(b))
	}
}

func mustGetwd() string {
	wd, _ := os.Getwd()
	return wd
}

// newStubRunner builds a Runner that executes this test binary as the stub.
// Stub control variables travel via the Runner's test-only env injection —
// the production minimalEnv never passes them through.
func newStubRunner(t *testing.T, mode string, hosts []HostSpec) *Runner {
	t.Helper()
	names := make([]string, 0, len(hosts))
	for _, h := range hosts {
		names = append(names, h.Name)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	return &Runner{
		Exe:            exe,
		Timeout:        30 * time.Second,
		testArgsPrefix: []string{"-test.run=TestHelperProcess", "--"},
		testExtraEnv: []string{
			"NODEX_ANSIBLE_STUB=1",
			"NODEX_ANSIBLE_STUB_MODE=" + mode,
			"NODEX_ANSIBLE_STUB_HOSTS=" + strings.Join(names, ","),
		},
	}
}

func testHosts() []HostSpec {
	return []HostSpec{
		{Name: "web1", Address: "web1.example.invalid", Port: 22, User: "automation", KeyFile: "/home/automation/.ssh/id_ed25519"},
		{Name: "db1", Address: "10.0.0.7", User: "automation"},
	}
}

func TestRunHappyPath(t *testing.T) {
	hosts := testHosts()
	r := newStubRunner(t, "ok", hosts)
	res, err := r.Run(context.Background(), RunRequest{Operation: "verify-host", Hosts: hosts})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.Success {
		t.Errorf("expected success, got %+v", res)
	}
	if res.PartialFailure {
		t.Error("no partial failure expected")
	}
	if len(res.Hosts) != 2 {
		t.Fatalf("expected 2 host results, got %d", len(res.Hosts))
	}
	for _, h := range res.Hosts {
		if h.Failed {
			t.Errorf("host %s unexpectedly failed", h.Host)
		}
	}
}

func TestRunRejectsUnknownOperation(t *testing.T) {
	r := newStubRunner(t, "ok", testHosts())
	_, err := r.Run(context.Background(), RunRequest{Operation: "rm-rf-everything", Hosts: testHosts()})
	if err == nil {
		t.Fatal("unknown operation must be rejected")
	}
	if !strings.Contains(err.Error(), "unknown maintenance operation") {
		t.Errorf("error = %v", err)
	}
}

func TestRunRejectsNoHosts(t *testing.T) {
	r := newStubRunner(t, "ok", nil)
	if _, err := r.Run(context.Background(), RunRequest{Operation: "verify-host"}); err == nil {
		t.Fatal("empty host list must be rejected")
	}
}

func TestRunRejectsMaliciousHostValues(t *testing.T) {
	bad := []HostSpec{
		{Name: "h1", Address: "addr ansible_shell_type=powershell", User: "automation"},
		{Name: "h1\nevil ansible_host=attacker", Address: "a.example.invalid", User: "automation"},
		{Name: "h1", Address: "a.example.invalid", User: "auto mation"},
		{Name: "h1", Address: "a.example.invalid", User: "automation", KeyFile: "/path/with space"},
		{Name: "h1", Address: "a.example.invalid", User: "automation", KnownHostsFile: "/kh\nevil=1"},
		{Name: "h1", Address: "a.example.invalid", User: "automation", Port: 99999},
	}
	for i, h := range bad {
		r := newStubRunner(t, "ok", []HostSpec{h})
		if _, err := r.Run(context.Background(), RunRequest{Operation: "verify-host", Hosts: []HostSpec{h}}); err == nil {
			t.Errorf("case %d: malicious host spec accepted: %+v", i, h)
		}
	}
}

func TestRunRejectsDuplicateHosts(t *testing.T) {
	h := HostSpec{Name: "web1", Address: "a.example.invalid", User: "automation"}
	r := newStubRunner(t, "ok", []HostSpec{h, h})
	if _, err := r.Run(context.Background(), RunRequest{Operation: "verify-host", Hosts: []HostSpec{h, h}}); err == nil {
		t.Fatal("duplicate hosts must be rejected")
	}
}

func TestRunPartialFailure(t *testing.T) {
	hosts := testHosts()
	r := newStubRunner(t, "one-failed", hosts)
	res, err := r.Run(context.Background(), RunRequest{Operation: "verify-host", Hosts: hosts})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Success {
		t.Error("run with a failed host must not be success")
	}
	if !res.PartialFailure {
		t.Error("one failed + one ok host must be a partial failure")
	}
	var failed, ok int
	for _, h := range res.Hosts {
		if h.Failed {
			failed++
		} else {
			ok++
		}
	}
	if failed != 1 || ok != 1 {
		t.Errorf("expected 1 failed + 1 ok, got %d/%d", failed, ok)
	}
}

func TestRunUnreachableHost(t *testing.T) {
	hosts := testHosts()
	r := newStubRunner(t, "unreachable", hosts)
	res, err := r.Run(context.Background(), RunRequest{Operation: "verify-host", Hosts: hosts})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Success {
		t.Error("unreachable host must not be success")
	}
	if !res.PartialFailure {
		t.Error("expected partial failure")
	}
}

// TestRunMissingHostNeverSuccess: a host absent from the stats must be
// treated as failed, even with exit code 0.
func TestRunMissingHostNeverSuccess(t *testing.T) {
	hosts := testHosts()
	r := newStubRunner(t, "missing-host", hosts)
	res, err := r.Run(context.Background(), RunRequest{Operation: "verify-host", Hosts: hosts})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Success {
		t.Error("missing host stats must not be success")
	}
	if res.ParseError == "" {
		t.Error("missing hosts should be reported in ParseError")
	}
}

// TestRunExitZeroButBadJSONNeverSuccess: success is never inferred from the
// exit code alone.
func TestRunBadJSONNeverSuccess(t *testing.T) {
	hosts := testHosts()
	r := newStubRunner(t, "bad-json", hosts)
	res, err := r.Run(context.Background(), RunRequest{Operation: "verify-host", Hosts: hosts})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Success {
		t.Error("unparseable output must not be success even with exit 0")
	}
	if res.ParseError == "" {
		t.Error("expected a parse error")
	}
}

func TestRunNonzeroExitWithCleanStatsNeverSuccess(t *testing.T) {
	hosts := testHosts()
	r := newStubRunner(t, "exit1-good-stats", hosts)
	res, err := r.Run(context.Background(), RunRequest{Operation: "verify-host", Hosts: hosts})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Success {
		t.Error("nonzero exit must not be success even with clean stats")
	}
}

func TestRunMinimalEnvironment(t *testing.T) {
	hosts := testHosts()[:1]
	r := newStubRunner(t, "env-dump", hosts)
	t.Setenv("NODEX_SECRET_SHOULD_NOT_LEAK", "sensitive")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "sensitive")
	res, err := r.Run(context.Background(), RunRequest{Operation: "verify-host", Hosts: hosts})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.Contains(res.Stdout, "NODEX_SECRET_SHOULD_NOT_LEAK") || strings.Contains(res.Stdout, "AWS_SECRET_ACCESS_KEY") {
		t.Error("parent environment leaked into the child process")
	}
	for _, want := range []string{"ANSIBLE_HOST_KEY_CHECKING=True", "ANSIBLE_STDOUT_CALLBACK=json", "ANSIBLE_CONFIG="} {
		if !strings.Contains(res.Stdout, want) {
			t.Errorf("child environment missing %q", want)
		}
	}
}

func TestRunBoundedOutput(t *testing.T) {
	hosts := testHosts()[:1]
	r := newStubRunner(t, "huge-output", hosts)
	r.MaxOutputBytes = 64 * 1024
	res, err := r.Run(context.Background(), RunRequest{Operation: "verify-host", Hosts: hosts})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.StdoutTruncated {
		t.Error("expected stdout truncation")
	}
	if int64(len(res.Stdout)) > 64*1024 {
		t.Errorf("stdout exceeds bound: %d bytes", len(res.Stdout))
	}
	if res.Success {
		t.Error("truncated output must not be success (results unverifiable)")
	}
}

func TestRunRedactsChildOutput(t *testing.T) {
	hosts := testHosts()[:1]
	r := newStubRunner(t, "secret-output", hosts)
	res, err := r.Run(context.Background(), RunRequest{Operation: "verify-host", Hosts: hosts})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.Contains(res.Stdout, "stub-secret-value") || strings.Contains(res.Stdout, "hunter2") {
		t.Errorf("child output not redacted: %q", res.Stdout)
	}
}

func TestRunCancellationKillsChild(t *testing.T) {
	hosts := testHosts()[:1]
	r := newStubRunner(t, "hang", hosts)
	r.Timeout = 2 * time.Second
	start := time.Now()
	_, err := r.Run(context.Background(), RunRequest{Operation: "verify-host", Hosts: hosts})
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out or was cancelled") {
		t.Errorf("error = %v", err)
	}
	if elapsed > 30*time.Second {
		t.Errorf("child not terminated promptly: %s", elapsed)
	}
}

func TestRunGeneratedFilesAndPrivateDir(t *testing.T) {
	hosts := []HostSpec{{
		Name: "web1", Address: "web1.example.invalid", Port: 2222,
		User: "automation", KeyFile: "/home/automation/.ssh/id_ed25519",
		KnownHostsFile: "/home/automation/.ssh/known_hosts_nodex",
	}}
	r := newStubRunner(t, "inspect-files", hosts)
	res, err := r.Run(context.Background(), RunRequest{Operation: "check-updates", Hosts: hosts})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var payload struct {
		Inventory string `json:"inventory"`
		Playbook  string `json:"playbook"`
		CWD       string `json:"cwd"`
	}
	if err := json.Unmarshal([]byte(res.Stdout), &payload); err != nil {
		t.Fatalf("parse stub payload: %v\n%s", err, res.Stdout)
	}
	for _, want := range []string{
		"web1 ansible_host=web1.example.invalid",
		"ansible_port=2222",
		"ansible_user=automation",
		"ansible_ssh_private_key_file=/home/automation/.ssh/id_ed25519",
		"-oUserKnownHostsFile=/home/automation/.ssh/known_hosts_nodex",
	} {
		if !strings.Contains(payload.Inventory, want) {
			t.Errorf("generated inventory missing %q:\n%s", want, payload.Inventory)
		}
	}
	if !strings.Contains(payload.Playbook, "Check for pending updates") {
		t.Error("embedded check-updates playbook not written")
	}
	if !strings.Contains(filepath.Base(payload.CWD), "nodex-ansible-") {
		t.Errorf("child did not run in the private work dir: %s", payload.CWD)
	}
	// The private directory must be gone after the run.
	if _, err := os.Stat(payload.CWD); !os.IsNotExist(err) {
		t.Errorf("private work dir not cleaned up: %s", payload.CWD)
	}
}

func TestRunCleansUpOnFailure(t *testing.T) {
	hosts := testHosts()[:1]
	r := newStubRunner(t, "hang", hosts)
	r.Timeout = 1 * time.Second
	before := countNodexTempDirs(t)
	_, _ = r.Run(context.Background(), RunRequest{Operation: "verify-host", Hosts: hosts})
	after := countNodexTempDirs(t)
	if after > before {
		t.Errorf("temp dirs leaked on failure: before=%d after=%d", before, after)
	}
}

func countNodexTempDirs(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir(os.TempDir())
	if err != nil {
		t.Fatalf("read temp dir: %v", err)
	}
	n := 0
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "nodex-ansible-") {
			n++
		}
	}
	return n
}

func TestRegistryAllowlist(t *testing.T) {
	ids := OperationIDs()
	if len(ids) != 2 || ids[0] != "check-updates" || ids[1] != "verify-host" {
		t.Errorf("unexpected allowlist: %v", ids)
	}
	for _, op := range Operations() {
		if !op.ReadOnly {
			t.Errorf("phase 4 operation %q must be read-only", op.ID)
		}
		if op.Playbook() == "" {
			t.Errorf("operation %q has no embedded playbook", op.ID)
		}
		if strings.Contains(op.Playbook(), "shell:") || strings.Contains(op.Playbook(), "ansible.builtin.shell") {
			t.Errorf("operation %q playbook uses the shell module", op.ID)
		}
	}
	if _, err := Lookup("install-security-updates"); err == nil {
		t.Error("operations not yet implemented must not resolve")
	}
}

func TestRunnerRequiresAbsoluteExe(t *testing.T) {
	r := &Runner{Exe: "ansible-playbook"}
	if _, err := r.Run(context.Background(), RunRequest{Operation: "verify-host", Hosts: testHosts()}); err == nil {
		t.Fatal("relative executable must be rejected")
	}
}

func TestVersionParsing(t *testing.T) {
	tests := []struct {
		line string
		ok   bool
	}{
		{"ansible-playbook [core 2.16.5]", true},
		{"ansible-playbook [core 2.12.0]", true},
		{"ansible-playbook [core 2.11.9]", false},
		{"ansible-playbook 2.9.6", false},
		{"garbage", false},
	}
	for _, tt := range tests {
		m := versionRe.FindStringSubmatch(tt.line)
		if tt.ok && m == nil && strings.Contains(tt.line, "core") {
			t.Errorf("version %q should parse", tt.line)
		}
		if m != nil {
			major := m[1]
			minor := m[2]
			tooOld := major == "2" && (minor == "11" || minor == "9")
			if tt.ok && tooOld {
				t.Errorf("case %q: expectation mismatch", tt.line)
			}
		}
	}
}

// TestTaskOutcomesParsed verifies task-level results reach the RunResult.
func TestTaskOutcomesParsed(t *testing.T) {
	hosts := testHosts()[:1]
	r := newStubRunner(t, "task-detail", hosts)
	res, err := r.Run(context.Background(), RunRequest{Operation: "check-updates", Hosts: hosts})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	outcomes := res.TaskOutcomes["web1"]
	if len(outcomes) != 3 {
		t.Fatalf("expected 3 task outcomes, got %d: %+v", len(outcomes), outcomes)
	}
	byName := map[string]TaskOutcome{}
	for _, o := range outcomes {
		byName[o.Task] = o
	}
	up := byName["List upgradable packages"]
	if len(up.StdoutLines) != 2 || up.StdoutLines[1] != "nano/stable 8.0-1 amd64 [upgradable from: 7.2-1]" {
		t.Errorf("upgradable stdout lines wrong: %+v", up)
	}
	rb := byName["Check reboot-required marker"]
	if rb.StatExists == nil || !*rb.StatExists {
		t.Errorf("reboot-required stat wrong: %+v", rb)
	}
	failed := byName["List failed systemd units"]
	if len(failed.StdoutLines) != 0 {
		t.Errorf("failed units should be empty: %+v", failed)
	}
}
