package ansible

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/geoffmcc/nodex/internal/output"
	"github.com/geoffmcc/nodex/internal/redact"
)

const (
	// DefaultTimeout bounds a whole playbook run.
	DefaultTimeout = 15 * time.Minute

	// DefaultMaxOutputBytes bounds captured stdout and stderr, each.
	DefaultMaxOutputBytes int64 = 4 * 1024 * 1024

	// terminateGrace is how long a cancelled child gets between SIGTERM and
	// SIGKILL.
	terminateGrace = 10 * time.Second

	// MinVersion is the oldest supported ansible-core version.
	MinVersionMajor = 2
	MinVersionMinor = 12
)

// ErrNotInstalled is returned when no usable ansible-playbook executable is
// found.
var ErrNotInstalled = fmt.Errorf("ansible-playbook is not installed or not on PATH")

// Detection describes a validated ansible-playbook executable.
type Detection struct {
	Path    string `json:"path" yaml:"path"`
	Version string `json:"version" yaml:"version"`
}

// versionRe matches the first line of `ansible-playbook --version`, e.g.
// "ansible-playbook [core 2.16.5]".
var versionRe = regexp.MustCompile(`\[core (\d+)\.(\d+)(?:\.(\d+))?\]`)

// Detect locates ansible-playbook on PATH, validates the executable path,
// and checks the version. The executable must resolve to an absolute path
// (Go's exec.LookPath already refuses relative results) that is not
// world-writable and does not live in a world-writable directory without
// the sticky bit.
func Detect(ctx context.Context) (*Detection, error) {
	path, err := exec.LookPath("ansible-playbook")
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNotInstalled, err)
	}
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("%w: resolved path %q is not absolute", ErrNotInstalled, path)
	}
	if err := checkExecutableSafety(path); err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, path, "--version") // #nosec G204 -- path from exec.LookPath, absolute, world-writability checked above
	cmd.Env = minimalEnv(nil)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ansible-playbook --version failed: %w", err)
	}
	m := versionRe.FindStringSubmatch(out.String())
	if m == nil {
		return nil, fmt.Errorf("could not parse ansible-playbook version from %q", firstLine(out.String()))
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	if major < MinVersionMajor || (major == MinVersionMajor && minor < MinVersionMinor) {
		return nil, fmt.Errorf("ansible-core %d.%d is too old (minimum %d.%d)", major, minor, MinVersionMajor, MinVersionMinor)
	}
	version := m[1] + "." + m[2]
	if m[3] != "" {
		version += "." + m[3]
	}
	return &Detection{Path: path, Version: version}, nil
}

// checkExecutableSafety rejects executables that are world-writable or that
// live in world-writable directories without the sticky bit.
func checkExecutableSafety(path string) error {
	if runtime.GOOS == "windows" {
		return nil // Unix permission bits are not meaningful here.
	}
	fi, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if fi.Mode().Perm()&0o002 != 0 {
		return fmt.Errorf("refusing world-writable executable %s", path)
	}
	dir := filepath.Dir(path)
	di, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("stat %s: %w", dir, err)
	}
	if di.Mode().Perm()&0o002 != 0 && di.Mode()&os.ModeSticky == 0 {
		return fmt.Errorf("refusing executable in world-writable directory %s", dir)
	}
	return nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// HostSpec describes one target host. Values are validated strictly because
// they are written into a generated inventory file.
type HostSpec struct {
	Name           string
	Address        string
	Port           int
	User           string
	KeyFile        string
	KnownHostsFile string
}

// inventoryValueRe restricts inventory values to characters that cannot
// break out of the generated INI line.
var inventoryValueRe = regexp.MustCompile(`^[A-Za-z0-9._~/:-]+$`)

func (h HostSpec) validate() error {
	if h.Name == "" || !inventoryValueRe.MatchString(h.Name) {
		return fmt.Errorf("invalid host name %q", h.Name)
	}
	if h.Address == "" || !inventoryValueRe.MatchString(h.Address) {
		return fmt.Errorf("host %s: invalid address", h.Name)
	}
	if h.User == "" || !inventoryValueRe.MatchString(h.User) {
		return fmt.Errorf("host %s: invalid user", h.Name)
	}
	if h.Port < 0 || h.Port > 65535 {
		return fmt.Errorf("host %s: invalid port %d", h.Name, h.Port)
	}
	for label, p := range map[string]string{"key file": h.KeyFile, "known_hosts file": h.KnownHostsFile} {
		if p == "" {
			continue
		}
		if !inventoryValueRe.MatchString(p) {
			return fmt.Errorf("host %s: %s path contains unsupported characters", h.Name, label)
		}
	}
	return nil
}

// RunRequest describes one allowlisted operation run.
type RunRequest struct {
	// Operation is an allowlisted operation ID (see registry.go).
	Operation string

	// Hosts are the explicit targets.
	Hosts []HostSpec
}

// HostResult is the per-host outcome parsed from Ansible's JSON callback.
type HostResult struct {
	Host        string `json:"host" yaml:"host"`
	OK          int    `json:"ok" yaml:"ok"`
	Changed     int    `json:"changed" yaml:"changed"`
	Failures    int    `json:"failures" yaml:"failures"`
	Unreachable int    `json:"unreachable" yaml:"unreachable"`
	Skipped     int    `json:"skipped" yaml:"skipped"`
	Failed      bool   `json:"failed" yaml:"failed"`
}

// TaskOutcome is one task's result on one host, extracted from the JSON
// callback. Fields cover the registered values Nodex's embedded playbooks
// produce; unknown fields are ignored.
type TaskOutcome struct {
	Task        string   `json:"task" yaml:"task"`
	Failed      bool     `json:"failed,omitempty" yaml:"failed,omitempty"`
	Skipped     bool     `json:"skipped,omitempty" yaml:"skipped,omitempty"`
	Unreachable bool     `json:"unreachable,omitempty" yaml:"unreachable,omitempty"`
	StdoutLines []string `json:"stdout_lines,omitempty" yaml:"stdout_lines,omitempty"`
	StatExists  *bool    `json:"stat_exists,omitempty" yaml:"stat_exists,omitempty"`
	Message     string   `json:"msg,omitempty" yaml:"msg,omitempty"`
}

// RunResult is the complete outcome of one adapter run. Success is derived
// from the parsed per-host statistics, never from the exit code alone.
type RunResult struct {
	Operation       string                   `json:"operation" yaml:"operation"`
	ExitCode        int                      `json:"exit_code" yaml:"exit_code"`
	DurationSeconds float64                  `json:"duration_seconds" yaml:"duration_seconds"`
	Hosts           []HostResult             `json:"hosts" yaml:"hosts"`
	TaskOutcomes    map[string][]TaskOutcome `json:"task_outcomes,omitempty" yaml:"task_outcomes,omitempty"`
	Success         bool                     `json:"success" yaml:"success"`
	PartialFailure  bool                     `json:"partial_failure" yaml:"partial_failure"`
	ParseError      string                   `json:"parse_error,omitempty" yaml:"parse_error,omitempty"`
	Stdout          string                   `json:"stdout,omitempty" yaml:"stdout,omitempty"`
	Stderr          string                   `json:"stderr,omitempty" yaml:"stderr,omitempty"`
	StdoutTruncated bool                     `json:"stdout_truncated,omitempty" yaml:"stdout_truncated,omitempty"`
	StderrTruncated bool                     `json:"stderr_truncated,omitempty" yaml:"stderr_truncated,omitempty"`
}

// Runner executes allowlisted operations through ansible-playbook.
type Runner struct {
	// Exe is the absolute path of the validated ansible-playbook executable
	// (from Detect).
	Exe string

	// Timeout bounds the run; DefaultTimeout when zero.
	Timeout time.Duration

	// MaxOutputBytes bounds captured stdout and stderr each;
	// DefaultMaxOutputBytes when zero.
	MaxOutputBytes int64

	// testArgsPrefix is prepended to the command arguments; used only by
	// package tests to route execution through the test binary.
	testArgsPrefix []string

	// testExtraEnv is appended to the child environment; used only by
	// package tests to control the stub executable.
	testExtraEnv []string
}

// Run executes one allowlisted operation. The child process gets a private
// 0700 working directory (removed afterward on every path), a generated
// inventory, the embedded playbook, a pinned ansible.cfg, and a minimal
// environment with SSH host-key checking enforced. Cancellation sends
// SIGTERM and escalates to SIGKILL after a grace period.
func (r *Runner) Run(ctx context.Context, req RunRequest) (*RunResult, error) {
	op, err := Lookup(req.Operation)
	if err != nil {
		return nil, err
	}
	if len(req.Hosts) == 0 {
		return nil, fmt.Errorf("no hosts specified")
	}
	seen := map[string]bool{}
	for _, h := range req.Hosts {
		if err := h.validate(); err != nil {
			return nil, err
		}
		if seen[h.Name] {
			return nil, fmt.Errorf("duplicate host %q", h.Name)
		}
		seen[h.Name] = true
	}
	if r.Exe == "" || !filepath.IsAbs(r.Exe) {
		return nil, fmt.Errorf("runner requires an absolute ansible-playbook path (run Detect first)")
	}

	timeout := r.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	maxOut := r.MaxOutputBytes
	if maxOut <= 0 {
		maxOut = DefaultMaxOutputBytes
	}

	// Private working directory; always removed.
	workDir, err := os.MkdirTemp("", "nodex-ansible-*")
	if err != nil {
		return nil, fmt.Errorf("create private work dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(workDir) }()
	if err := os.Chmod(workDir, 0o700); err != nil { // #nosec G302 -- directories need the owner execute bit; 0700 is owner-only
		return nil, fmt.Errorf("restrict work dir: %w", err)
	}

	playbookPath := filepath.Join(workDir, "playbook.yml")
	if err := os.WriteFile(playbookPath, []byte(op.Playbook()), 0o600); err != nil {
		return nil, fmt.Errorf("write playbook: %w", err)
	}
	inventoryPath := filepath.Join(workDir, "inventory.ini")
	if err := os.WriteFile(inventoryPath, []byte(renderInventory(req.Hosts)), 0o600); err != nil {
		return nil, fmt.Errorf("write inventory: %w", err)
	}
	// Pin the Ansible configuration so a working-directory ansible.cfg can
	// never inject plugins or weaken host-key checking.
	cfgPath := filepath.Join(workDir, "ansible.cfg")
	if err := os.WriteFile(cfgPath, []byte(pinnedAnsibleCfg), 0o600); err != nil {
		return nil, fmt.Errorf("write ansible.cfg: %w", err)
	}
	localTmp := filepath.Join(workDir, "tmp")
	if err := os.Mkdir(localTmp, 0o700); err != nil {
		return nil, fmt.Errorf("create ansible tmp: %w", err)
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := append(append([]string{}, r.testArgsPrefix...), "-i", inventoryPath, playbookPath)
	cmd := exec.CommandContext(runCtx, r.Exe, args...) // #nosec G204 -- executable is a Detect-validated absolute path; args are generated files, never user input
	cmd.Dir = workDir
	cmd.Env = append(minimalEnv(map[string]string{
		"ANSIBLE_CONFIG":     cfgPath,
		"ANSIBLE_LOCAL_TEMP": localTmp,
	}), r.testExtraEnv...)
	cmd.Cancel = func() error {
		if runtime.GOOS == "windows" {
			return cmd.Process.Kill()
		}
		return cmd.Process.Signal(syscall.SIGTERM)
	}
	cmd.WaitDelay = terminateGrace

	stdout := newBoundedBuffer(maxOut)
	stderr := newBoundedBuffer(maxOut)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	start := time.Now()
	runErr := cmd.Run()
	duration := time.Since(start)

	result := &RunResult{
		Operation:       req.Operation,
		DurationSeconds: duration.Seconds(),
		Stdout:          sanitizeOutput(stdout.String()),
		Stderr:          sanitizeOutput(stderr.String()),
		StdoutTruncated: stdout.truncated,
		StderrTruncated: stderr.truncated,
	}
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	} else {
		result.ExitCode = -1
	}

	if runCtx.Err() != nil {
		return result, fmt.Errorf("operation %q timed out or was cancelled after %s: %w", req.Operation, duration.Round(time.Second), runCtx.Err())
	}
	var exitErr *exec.ExitError
	if runErr != nil && !errors.As(runErr, &exitErr) {
		return result, fmt.Errorf("execute ansible-playbook: %w", runErr)
	}

	parseRunStats(result, stdout.Bytes(), req.Hosts)
	return result, nil
}

// parseRunStats extracts per-host statistics from the JSON callback output
// and derives success honestly: every requested host must appear with no
// failures and no unreachability, and the process must have exited zero.
func parseRunStats(result *RunResult, stdoutRaw []byte, hosts []HostSpec) {
	if result.StdoutTruncated {
		result.ParseError = "stdout truncated; per-host results unavailable"
		result.Success = false
		return
	}
	var payload struct {
		Stats map[string]struct {
			OK          int `json:"ok"`
			Changed     int `json:"changed"`
			Failures    int `json:"failures"`
			Unreachable int `json:"unreachable"`
			Skipped     int `json:"skipped"`
		} `json:"stats"`
		Plays []struct {
			Tasks []struct {
				Task struct {
					Name string `json:"name"`
				} `json:"task"`
				Hosts map[string]struct {
					Failed      bool     `json:"failed"`
					Skipped     bool     `json:"skipped"`
					Unreachable bool     `json:"unreachable"`
					StdoutLines []string `json:"stdout_lines"`
					Msg         any      `json:"msg"`
					Stat        *struct {
						Exists bool `json:"exists"`
					} `json:"stat"`
				} `json:"hosts"`
			} `json:"tasks"`
		} `json:"plays"`
	}
	if err := json.Unmarshal(stdoutRaw, &payload); err != nil || payload.Stats == nil {
		result.ParseError = "could not parse ansible JSON output"
		result.Success = false
		return
	}

	// Task-level outcomes for the embedded playbooks' registered results.
	result.TaskOutcomes = map[string][]TaskOutcome{}
	for _, play := range payload.Plays {
		for _, task := range play.Tasks {
			for host, hr := range task.Hosts {
				outcome := TaskOutcome{
					Task:        task.Task.Name,
					Failed:      hr.Failed,
					Skipped:     hr.Skipped,
					Unreachable: hr.Unreachable,
					StdoutLines: hr.StdoutLines,
				}
				if hr.Stat != nil {
					exists := hr.Stat.Exists
					outcome.StatExists = &exists
				}
				if s, ok := hr.Msg.(string); ok {
					outcome.Message = s
				}
				result.TaskOutcomes[host] = append(result.TaskOutcomes[host], outcome)
			}
		}
	}

	failedOrUnreachable := 0
	completed := 0
	for name, s := range payload.Stats {
		hr := HostResult{
			Host: name, OK: s.OK, Changed: s.Changed,
			Failures: s.Failures, Unreachable: s.Unreachable, Skipped: s.Skipped,
			Failed: s.Failures > 0 || s.Unreachable > 0,
		}
		result.Hosts = append(result.Hosts, hr)
		if hr.Failed {
			failedOrUnreachable++
		} else {
			completed++
		}
	}
	sort.Slice(result.Hosts, func(i, j int) bool { return result.Hosts[i].Host < result.Hosts[j].Host })

	missing := 0
	for _, h := range hosts {
		if _, ok := payload.Stats[h.Name]; !ok {
			missing++
			result.Hosts = append(result.Hosts, HostResult{Host: h.Name, Failed: true, Unreachable: 1})
		}
	}
	if missing > 0 {
		result.ParseError = fmt.Sprintf("%d requested host(s) missing from results", missing)
	}

	result.PartialFailure = (failedOrUnreachable > 0 || missing > 0) && completed > 0
	result.Success = result.ExitCode == 0 && failedOrUnreachable == 0 && missing == 0 && completed == len(hosts)
}

// renderInventory generates the INI inventory for validated host specs.
func renderInventory(hosts []HostSpec) string {
	var b strings.Builder
	b.WriteString("[nodex]\n")
	for _, h := range hosts {
		b.WriteString(h.Name)
		b.WriteString(" ansible_host=")
		b.WriteString(h.Address)
		if h.Port > 0 {
			b.WriteString(" ansible_port=")
			b.WriteString(strconv.Itoa(h.Port))
		}
		b.WriteString(" ansible_user=")
		b.WriteString(h.User)
		if h.KeyFile != "" {
			b.WriteString(" ansible_ssh_private_key_file=")
			b.WriteString(h.KeyFile)
		}
		if h.KnownHostsFile != "" {
			b.WriteString(" ansible_ssh_common_args=-oUserKnownHostsFile=")
			b.WriteString(h.KnownHostsFile)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// pinnedAnsibleCfg is written into the private work directory and selected
// via ANSIBLE_CONFIG, preventing any ambient ansible.cfg from injecting
// plugins or weakening safety settings.
const pinnedAnsibleCfg = `[defaults]
host_key_checking = True
retry_files_enabled = False
stdout_callback = json
nocows = 1
interpreter_python = auto_silent
[ssh_connection]
`

// minimalEnv builds the allowlisted child environment: only PATH and HOME
// pass through from the parent (both required for ssh), everything else is
// pinned by Nodex.
func minimalEnv(extra map[string]string) []string {
	env := []string{
		"LANG=C.UTF-8",
		"ANSIBLE_HOST_KEY_CHECKING=True",
		"ANSIBLE_STDOUT_CALLBACK=json",
		"ANSIBLE_RETRY_FILES_ENABLED=False",
		"ANSIBLE_NOCOLOR=1",
		"ANSIBLE_FORCE_COLOR=0",
	}
	for _, key := range []string{"PATH", "HOME", "USERPROFILE", "SYSTEMROOT", "TEMP", "TMP"} {
		if v, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+v)
		}
	}
	keys := make([]string, 0, len(extra))
	for k := range extra {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		env = append(env, k+"="+extra[k])
	}
	return env
}

// sanitizeOutput redacts secrets and strips terminal escape sequences from
// captured child output.
func sanitizeOutput(s string) string {
	return redact.String(output.SanitizeTerminal(s))
}

// boundedBuffer captures up to limit bytes and records truncation.
type boundedBuffer struct {
	buf       bytes.Buffer
	limit     int64
	truncated bool
}

func newBoundedBuffer(limit int64) *boundedBuffer {
	return &boundedBuffer{limit: limit}
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	remaining := b.limit - int64(b.buf.Len())
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil // discard, but report consumed to keep the pipe drained
	}
	if int64(len(p)) > remaining {
		b.truncated = true
		b.buf.Write(p[:remaining])
		return len(p), nil
	}
	b.buf.Write(p)
	return len(p), nil
}

func (b *boundedBuffer) String() string { return b.buf.String() }
func (b *boundedBuffer) Bytes() []byte  { return b.buf.Bytes() }
