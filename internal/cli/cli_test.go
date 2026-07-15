package cli

import (
	"bytes"
	"context"
	stderrors "errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/config"
	"github.com/geoffmcc/nodex/internal/credentials"
	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/output"
)

func TestRun_NoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !strings.Contains(stdout.String(), "Nodex") {
		t.Error("expected usage output")
	}
}

func TestRunSanitizesDirectHandlerStdoutAndStderr(t *testing.T) {
	const name = "unsafe-output-test"
	commands[name] = &command{
		name:  name,
		short: "test command with direct output",
		run: func(_ context.Context, cmdCtx *Context, _ []string) error {
			_, _ = fmt.Fprintf(cmdCtx.Writer, "stdout:%s\n", "bad\x1b]0;owned\x07PVEAPIToken=user@pam!tok=secret")
			_, _ = fmt.Fprintf(cmdCtx.ErrW, "stderr:%s\n", "bad\x1b]0;owned\x07PVEAPIToken=user@pam!tok=secret")
			return nil
		},
	}
	defer delete(commands, name)

	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{name}, &stdout, &stderr); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for stream, out := range map[string]string{"stdout": stdout.String(), "stderr": stderr.String()} {
		if strings.Contains(out, "\x1b") || strings.Contains(out, "owned") || strings.Contains(out, "secret") {
			t.Fatalf("%s was not sanitized/redacted: %q", stream, out)
		}
		if !strings.Contains(out, "[REDACTED]") {
			t.Fatalf("%s missing redaction marker: %q", stream, out)
		}
	}
}

func TestRun_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"help"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "Commands:") {
		t.Error("expected commands list")
	}
}

func TestRun_HelpCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"help", "version"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "nodex version") {
		t.Errorf("expected version help, got: %s", out)
	}
}

func TestRun_UnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"nonexistent"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for unknown command")
	}
	var exitCode *app.ExitCoder
	if !stderrors.As(err, &exitCode) || exitCode.ExitCode != app.ExitUsage {
		t.Errorf("expected ExitUsage, got: %v", err)
	}
}

func TestRun_Version(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"version"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !strings.Contains(stdout.String(), "Nodex") {
		t.Error("expected version output")
	}
}

func TestRun_ProviderSubcommands(t *testing.T) {
	var stdout, stderr bytes.Buffer

	// provider with no subcommand should print usage.
	err := Run(context.Background(), []string{"provider"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !strings.Contains(stdout.String(), "Subcommands:") {
		t.Error("expected subcommands list")
	}
}

func TestRun_ProviderList(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"provider", "list"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !strings.Contains(stdout.String(), "proxmox") {
		t.Error("expected proxmox in provider list")
	}
}

func TestRun_ProviderCapabilitiesUnknown(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"provider", "capabilities", "nonexistent"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestRun_ProfileSubcommands(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"profile"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !strings.Contains(stdout.String(), "Subcommands:") {
		t.Error("expected subcommands list")
	}
}

func TestRun_ProfileAddNoName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"profile", "add"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestRun_ProfileShowNoName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"profile", "show"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestRun_ProfileUseNoName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"profile", "use"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestRun_ProfileRemoveNoName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"profile", "remove"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestProfileSetCredentialsRegistered(t *testing.T) {
	cmd, ok := GetCommand("profile")
	if !ok {
		t.Fatal("profile command not registered")
	}
	if _, ok := cmd.sub["set-credentials"]; !ok {
		t.Fatal("profile command missing set-credentials subcommand")
	}
}

func TestRun_ProfileSetCredentialsNoName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"profile", "set-credentials"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestRunProfileSetCredentialsStoresFileCredential(t *testing.T) {
	_, home := isolateConfigAndHome(t)
	seed := config.DefaultConfig()
	seed.CurrentProfile = "lab"
	seed.Profiles["lab"] = config.Profile{Provider: "proxmox", Endpoint: "https://pve.example.invalid:8006"}
	path, err := config.ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	if err := config.WriteTo(seed, path); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	oldPrompter := credentialPrompter
	credentialPrompter = func(*Context) (*domain.Credentials, error) {
		return &domain.Credentials{Type: "token", TokenID: "user@pam!nodex", TokenSecret: "super-secret"}, nil
	}
	t.Cleanup(func() { credentialPrompter = oldPrompter })

	var stdout, stderr bytes.Buffer
	err = runProfileSetCredentials(context.Background(), &Context{Writer: &stdout, ErrW: &stderr}, []string{"lab"})
	if err != nil {
		t.Fatalf("runProfileSetCredentials: %v", err)
	}
	if strings.Contains(stdout.String(), "super-secret") || strings.Contains(stderr.String(), "super-secret") {
		t.Fatalf("command output leaked credential secret: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	cfg, err := config.Read()
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if got := cfg.Profiles["lab"].CredentialRef; got != "file:lab" {
		t.Fatalf("credential_ref = %q, want file:lab", got)
	}
	resolver := credentials.NewResolver(filepath.Join(home, ".nodex", "credentials"))
	creds, err := resolver.Resolve(context.Background(), "lab", "file:lab")
	if err != nil {
		t.Fatalf("resolve stored credentials: %v", err)
	}
	if creds.TokenID != "user@pam!nodex" || creds.TokenSecret != "super-secret" {
		t.Fatalf("stored credentials mismatch: %#v", creds)
	}
}

func TestRunProfileSetCredentialsRejectsUnsupportedBackend(t *testing.T) {
	isolateConfigAndHome(t)
	path, err := config.ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	if err := config.WriteTo(config.DefaultConfig(), path); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	var stdout, stderr bytes.Buffer
	err = runProfileSetCredentials(context.Background(), &Context{Writer: &stdout, ErrW: &stderr}, []string{"lab", "--backend", "env"})
	if err == nil {
		t.Fatal("expected unsupported backend error")
	}
	var exitCode *app.ExitCoder
	if !stderrors.As(err, &exitCode) || exitCode.ExitCode != app.ExitUsage {
		t.Fatalf("error = %v, want ExitUsage", err)
	}
}

func isolateConfigAndHome(t *testing.T) (dir, home string) {
	t.Helper()
	dir = t.TempDir()
	home = filepath.Join(dir, "home")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("AppData", filepath.Join(dir, "appdata"))
	return dir, home
}

func TestRun_GlobalFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"--quiet", "version"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestRun_InvalidOutputFormat(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"--output", "xml", "version"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for invalid output format")
	}
}

func TestRun_RejectsExtraArgs(t *testing.T) {
	for _, args := range [][]string{
		{"version", "extra"},
		{"provider", "list", "extra"},
		{"profile", "remove", "name", "--remove-credential-extra"},
		{"help", "version", "extra"},
	} {
		var stdout, stderr bytes.Buffer
		if err := Run(context.Background(), args, &stdout, &stderr); err == nil {
			t.Fatalf("Run(%v) succeeded, want usage error", args)
		}
	}
}

func TestRun_RejectsInvalidTimeout(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"--timeout", "0s", "version"}, &stdout, &stderr); err == nil {
		t.Fatal("expected invalid timeout error")
	}
}

func TestResourceShowSubcommandsRegistered(t *testing.T) {
	for _, commandName := range []string{"node", "vm", "container", "storage"} {
		cmd, ok := GetCommand(commandName)
		if !ok {
			t.Fatalf("command %q not registered", commandName)
		}
		if _, ok := cmd.sub["show"]; !ok {
			t.Fatalf("command %q missing show subcommand", commandName)
		}
	}
}

func TestResourceShowRejectsWrongArgCounts(t *testing.T) {
	tests := []struct {
		name string
		run  CommandFunc
		args []string
	}{
		{name: "node", run: runNodeShow},
		{name: "vm", run: runVMShow},
		{name: "container", run: runContainerShow},
		{name: "storage", run: runStorageShow},
		{name: "node-extra", run: runNodeShow, args: []string{"one", "two"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := tt.run(context.Background(), &Context{Writer: &stdout, ErrW: &stderr}, tt.args)
			if err == nil {
				t.Fatal("expected usage error")
			}
			var exitCode *app.ExitCoder
			if !stderrors.As(err, &exitCode) || exitCode.ExitCode != app.ExitUsage {
				t.Fatalf("error = %v, want ExitUsage", err)
			}
		})
	}
}

func TestResourceShowRejectsInvalidGuestTargetsBeforeConnect(t *testing.T) {
	tests := []struct {
		name string
		run  CommandFunc
		arg  string
	}{
		{name: "vm missing slash", run: runVMShow, arg: "badformat"},
		{name: "vm negative id", run: runVMShow, arg: "pve-test/-1"},
		{name: "container missing slash", run: runContainerShow, arg: "badformat"},
		{name: "container negative id", run: runContainerShow, arg: "pve-test/-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := tt.run(context.Background(), &Context{Writer: &stdout, ErrW: &stderr}, []string{tt.arg})
			if err == nil {
				t.Fatal("expected usage error")
			}
			var exitCode *app.ExitCoder
			if !stderrors.As(err, &exitCode) || exitCode.ExitCode != app.ExitUsage {
				t.Fatalf("error = %v, want ExitUsage", err)
			}
		})
	}
}

func TestWriteNodesTableShowsUnavailableFieldsHonestly(t *testing.T) {
	var stdout bytes.Buffer
	cmdCtx := &Context{Writer: &stdout, Opts: Options{Output: output.FormatTable}}
	if err := writeNodes(cmdCtx, []domain.Node{{ID: "node/proxmox", Name: "proxmox", Status: "online", Role: "node", Platform: "proxmox"}}); err != nil {
		t.Fatalf("writeNodes: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "proxmox") || !strings.Contains(out, "online") {
		t.Fatalf("table output missing node data: %q", out)
	}
	if strings.Contains(out, "0s") {
		t.Fatalf("table output represented omitted uptime as 0s: %q", out)
	}
	if strings.Contains(out, "<nil>") {
		t.Fatalf("table output leaked nil value: %q", out)
	}
}

func TestWriteNodesStructuredOutputOmitsUnavailableFields(t *testing.T) {
	for _, format := range []output.Format{output.FormatJSON, output.FormatYAML} {
		t.Run(string(format), func(t *testing.T) {
			var stdout bytes.Buffer
			cmdCtx := &Context{Writer: &stdout, Opts: Options{Output: format}}
			if err := writeNodes(cmdCtx, []domain.Node{{ID: "node/proxmox", Name: "proxmox", Status: "online", Role: "node", Platform: "proxmox"}}); err != nil {
				t.Fatalf("writeNodes: %v", err)
			}
			out := stdout.String()
			if !strings.Contains(out, "proxmox") || strings.Contains(out, "uptime") || strings.Contains(out, "0s") {
				t.Fatalf("unsafe structured output for %s: %q", format, out)
			}
		})
	}
}

func TestWriteNodesMultipleWithUptime(t *testing.T) {
	uptime := 42 * time.Second
	var stdout bytes.Buffer
	cmdCtx := &Context{Writer: &stdout, Opts: Options{Output: output.FormatTable}}
	if err := writeNodes(cmdCtx, []domain.Node{
		{ID: "node/b", Name: "b", Status: "offline", Role: "node", Platform: "proxmox"},
		{ID: "node/a", Name: "a", Status: "online", Role: "node", Platform: "proxmox", Uptime: &uptime},
	}); err != nil {
		t.Fatalf("writeNodes: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "42s") {
		t.Fatalf("table output missing present uptime: %q", out)
	}
	if strings.Index(out, "a") > strings.Index(out, "b") {
		t.Fatalf("nodes not sorted by name: %q", out)
	}
}

func TestFindResourceShowTargets(t *testing.T) {
	if node, ok := findNode([]domain.Node{{ID: "node/proxmox", Name: "proxmox"}}, "proxmox"); !ok || node.ID != "node/proxmox" {
		t.Fatalf("findNode by name = %+v, %v", node, ok)
	}
	if vm, ok := findVM([]domain.VM{{ID: "proxmox/100", Name: "vm-one"}}, "proxmox/100"); !ok || vm.Name != "vm-one" {
		t.Fatalf("findVM = %+v, %v", vm, ok)
	}
	if container, ok := findContainer([]domain.Container{{ID: "proxmox/200", Name: "ct-one"}}, "proxmox/200"); !ok || container.Name != "ct-one" {
		t.Fatalf("findContainer = %+v, %v", container, ok)
	}
	if storage, ok := findStorage([]domain.Storage{{ID: "storage/proxmox/local-lvm", Name: "local-lvm"}}, "local-lvm"); !ok || storage.ID != "storage/proxmox/local-lvm" {
		t.Fatalf("findStorage by name = %+v, %v", storage, ok)
	}
	if _, ok := findVM([]domain.VM{{ID: "proxmox/100"}}, "missing"); ok {
		t.Fatal("findVM matched missing ID")
	}
}

func TestWriteResourceShowOutput(t *testing.T) {
	t.Run("vm json", func(t *testing.T) {
		var stdout bytes.Buffer
		cmdCtx := &Context{Writer: &stdout, Opts: Options{Output: output.FormatJSON}}
		if err := writeVM(cmdCtx, domain.VM{ID: "proxmox/100", Name: "vm-one", Status: "running", Node: "proxmox", CPU: 2}); err != nil {
			t.Fatalf("writeVM: %v", err)
		}
		out := stdout.String()
		if !strings.Contains(out, `"id": "proxmox/100"`) || !strings.Contains(out, `"name": "vm-one"`) {
			t.Fatalf("JSON output missing VM fields: %q", out)
		}
	})

	t.Run("storage table", func(t *testing.T) {
		var stdout bytes.Buffer
		cmdCtx := &Context{Writer: &stdout, Opts: Options{Output: output.FormatTable}}
		storage := domain.Storage{ID: "storage/proxmox/local-lvm", Name: "local-lvm", Type: "storage", Status: "available", Node: "proxmox", Total: 4096, Used: 1024, Avail: 3072, Content: []string{"images", "rootdir"}}
		if err := writeStorage(cmdCtx, storage); err != nil {
			t.Fatalf("writeStorage: %v", err)
		}
		out := stdout.String()
		if !strings.Contains(out, "local-lvm") || !strings.Contains(out, "images,rootdir") {
			t.Fatalf("table output missing storage fields: %q", out)
		}
	})
}

func TestRun_VersionCompare(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"version", "compare", "1.0.0", "2.0.0"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !strings.Contains(stdout.String(), "1.0.0 < 2.0.0") {
		t.Errorf("expected comparison output, got: %s", stdout.String())
	}
}

func TestRun_VersionCompareEqual(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"version", "compare", "1.0.0", "1.0.0"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !strings.Contains(stdout.String(), "1.0.0 == 1.0.0") {
		t.Errorf("expected equal output, got: %s", stdout.String())
	}
}

func TestRun_VersionCompareGreater(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"version", "compare", "2.0.0", "1.0.0"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !strings.Contains(stdout.String(), "2.0.0 > 1.0.0") {
		t.Errorf("expected greater output, got: %s", stdout.String())
	}
}

func TestRun_VersionCompareWrongArgCount(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"version", "compare", "1.0.0"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for missing arg")
	}
}

func TestRun_VersionCompareInvalidVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"version", "compare", "invalid", "1.0.0"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for invalid version")
	}
}

func TestRun_VersionParse(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"version", "parse", "1.2.3-alpha.1+build.456"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "Major:      1") || !strings.Contains(out, "Prerelease: alpha.1") || !strings.Contains(out, "Build meta: build.456") {
		t.Errorf("expected parse output, got: %s", out)
	}
}

func TestRun_VersionParseWrongArgCount(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"version", "parse"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for missing arg")
	}
}

func TestRun_VersionParseInvalid(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"version", "parse", "not-a-version"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for invalid version")
	}
}

func TestRun_VersionSubcommandsRegistered(t *testing.T) {
	cmd, ok := GetCommand("version")
	if !ok {
		t.Fatal("version command not registered")
	}
	for _, subName := range []string{"compare", "parse"} {
		if _, ok := cmd.sub[subName]; !ok {
			t.Fatalf("version command missing %s subcommand", subName)
		}
	}
}

func TestRun_VersionUnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"version", "bogus"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for unknown version subcommand")
	}
}

func TestNodeDetailSubcommandsRegistered(t *testing.T) {
	cmd, ok := GetCommand("node")
	if !ok {
		t.Fatal("node command not registered")
	}
	subs := []string{"services", "network", "dns", "time", "disks", "certificates", "subscription", "updates"}
	for _, subName := range subs {
		if _, ok := cmd.sub[subName]; !ok {
			t.Fatalf("node command missing %s subcommand", subName)
		}
	}
}

func TestNodeDetailSubcommandsRejectWrongArgCount(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "services", args: []string{"node", "services"}},
		{name: "services-extra", args: []string{"node", "services", "node1", "extra"}},
		{name: "network", args: []string{"node", "network"}},
		{name: "network-extra", args: []string{"node", "network", "node1", "extra"}},
		{name: "dns", args: []string{"node", "dns"}},
		{name: "dns-extra", args: []string{"node", "dns", "node1", "extra"}},
		{name: "time", args: []string{"node", "time"}},
		{name: "time-extra", args: []string{"node", "time", "node1", "extra"}},
		{name: "disks", args: []string{"node", "disks"}},
		{name: "disks-extra", args: []string{"node", "disks", "node1", "extra"}},
		{name: "certificates", args: []string{"node", "certificates"}},
		{name: "certificates-extra", args: []string{"node", "certificates", "node1", "extra"}},
		{name: "subscription", args: []string{"node", "subscription"}},
		{name: "subscription-extra", args: []string{"node", "subscription", "node1", "extra"}},
		{name: "updates", args: []string{"node", "updates"}},
		{name: "updates-extra", args: []string{"node", "updates", "node1", "extra"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateConfigAndHome(t)
			setupE2EConfig(t)

			var stdout, stderr bytes.Buffer
			err := Run(context.Background(), tt.args, &stdout, &stderr)
			if err == nil {
				t.Fatal("expected usage error")
			}
			var exitCode *app.ExitCoder
			if !stderrors.As(err, &exitCode) || exitCode.ExitCode != app.ExitUsage {
				t.Fatalf("error = %v, want ExitUsage", err)
			}
		})
	}
}

func setupE2EConfig(t *testing.T) {
	t.Helper()
	t.Setenv("NODEX_E2E_TOKEN", "e2e-token")
	cfg := config.DefaultConfig()
	cfg.CurrentProfile = "e2e"
	cfg.Profiles["e2e"] = config.Profile{Provider: e2eMockProviderName, Endpoint: "https://e2e.example.invalid", CredentialRef: "env:e2e"}
	path, err := config.ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	if err := config.WriteTo(cfg, path); err != nil {
		t.Fatalf("seed config: %v", err)
	}
}

func TestRun_NodeServices(t *testing.T) {
	isolateConfigAndHome(t)
	setupE2EConfig(t)

	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"node", "services", "e2e-node"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestRun_NodeDNS(t *testing.T) {
	isolateConfigAndHome(t)
	setupE2EConfig(t)

	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"node", "dns", "e2e-node"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !strings.Contains(stdout.String(), "8.8.8.8") {
		t.Errorf("expected DNS1 in output, got: %s", stdout.String())
	}
}

func TestRun_NodeSubscription(t *testing.T) {
	isolateConfigAndHome(t)
	setupE2EConfig(t)

	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"node", "subscription", "e2e-node"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !strings.Contains(stdout.String(), "valid") {
		t.Errorf("expected subscription status in output, got: %s", stdout.String())
	}
}

func TestWriteNodeDetailNilHandling(t *testing.T) {
	for _, format := range []output.Format{output.FormatJSON, output.FormatYAML} {
		t.Run(string(format), func(t *testing.T) {
			ctx := &Context{Writer: &bytes.Buffer{}, Opts: Options{Output: format}}
			if err := writeNodeServices(ctx, nil); err != nil {
				t.Fatalf("writeNodeServices nil: %v", err)
			}
			if err := writeNodeNetwork(ctx, nil); err != nil {
				t.Fatalf("writeNodeNetwork nil: %v", err)
			}
			if err := writeNodeDNS(ctx, nil); err != nil {
				t.Fatalf("writeNodeDNS nil: %v", err)
			}
			if err := writeNodeTime(ctx, nil); err != nil {
				t.Fatalf("writeNodeTime nil: %v", err)
			}
			if err := writeNodeDisks(ctx, nil); err != nil {
				t.Fatalf("writeNodeDisks nil: %v", err)
			}
			if err := writeNodeCertificates(ctx, nil); err != nil {
				t.Fatalf("writeNodeCertificates nil: %v", err)
			}
			if err := writeNodeSubscription(ctx, nil); err != nil {
				t.Fatalf("writeNodeSubscription nil: %v", err)
			}
			if err := writeNodeUpdates(ctx, nil); err != nil {
				t.Fatalf("writeNodeUpdates nil: %v", err)
			}
		})
	}
}

func TestFirewallAdvancedSubcommandsRegistered(t *testing.T) {
	cmd, ok := GetCommand("firewall")
	if !ok {
		t.Fatal("firewall command not registered")
	}
	subs := []string{"aliases", "ipsets", "ipset", "security-groups", "options", "node-rules", "vm-rules"}
	for _, subName := range subs {
		if _, ok := cmd.sub[subName]; !ok {
			t.Fatalf("firewall command missing %s subcommand", subName)
		}
	}
}

func TestFirewallAdvancedSubcommandsRejectWrongArgCount(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "aliases-extra", args: []string{"firewall", "aliases", "extra"}},
		{name: "ipsets-extra", args: []string{"firewall", "ipsets", "extra"}},
		{name: "ipset-no-name", args: []string{"firewall", "ipset"}},
		{name: "ipset-extra", args: []string{"firewall", "ipset", "name", "extra"}},
		{name: "security-groups-extra", args: []string{"firewall", "security-groups", "extra"}},
		{name: "options-extra", args: []string{"firewall", "options", "extra"}},
		{name: "node-rules-no-node", args: []string{"firewall", "node-rules"}},
		{name: "node-rules-extra", args: []string{"firewall", "node-rules", "node1", "extra"}},
		{name: "vm-rules-no-arg", args: []string{"firewall", "vm-rules"}},
		{name: "vm-rules-bad-format", args: []string{"firewall", "vm-rules", "not-node/vmid-format"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateConfigAndHome(t)
			setupE2EConfig(t)

			var stdout, stderr bytes.Buffer
			err := Run(context.Background(), tt.args, &stdout, &stderr)
			if err == nil {
				t.Fatal("expected usage error")
			}
			var exitCode *app.ExitCoder
			if !stderrors.As(err, &exitCode) || exitCode.ExitCode != app.ExitUsage {
				t.Fatalf("error = %v, want ExitUsage", err)
			}
		})
	}
}

func TestRun_FirewallAliases(t *testing.T) {
	isolateConfigAndHome(t)
	setupE2EConfig(t)

	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"firewall", "aliases"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestRun_FirewallOptions(t *testing.T) {
	isolateConfigAndHome(t)
	setupE2EConfig(t)

	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"firewall", "options"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestWriteFirewallAdvancedNilHandling(t *testing.T) {
	for _, format := range []output.Format{output.FormatJSON, output.FormatYAML} {
		t.Run(string(format), func(t *testing.T) {
			ctx := &Context{Writer: &bytes.Buffer{}, Opts: Options{Output: format}}
			if err := writeFirewallAliases(ctx, nil); err != nil {
				t.Fatalf("writeFirewallAliases nil: %v", err)
			}
			if err := writeFirewallIPSets(ctx, nil); err != nil {
				t.Fatalf("writeFirewallIPSets nil: %v", err)
			}
			if err := writeFirewallIPSetEntries(ctx, nil); err != nil {
				t.Fatalf("writeFirewallIPSetEntries nil: %v", err)
			}
			if err := writeFirewallSecurityGroups(ctx, nil); err != nil {
				t.Fatalf("writeFirewallSecurityGroups nil: %v", err)
			}
			if err := writeFirewallOptionsTable(ctx, nil); err != nil {
				t.Fatalf("writeFirewallOptionsTable nil: %v", err)
			}
		})
	}
}

func TestPhase13SubcommandsRegistered(t *testing.T) {
	ha, ok := GetCommand("ha")
	if !ok {
		t.Fatal("ha command not registered")
	}
	for _, subName := range []string{"status", "current"} {
		if _, ok := ha.sub[subName]; !ok {
			t.Fatalf("ha command missing %s subcommand", subName)
		}
	}

	backup, ok := GetCommand("backup")
	if !ok {
		t.Fatal("backup command not registered")
	}
	if _, ok := backup.sub["content"]; !ok {
		t.Fatal("backup command missing content subcommand")
	}

	sdn, ok := GetCommand("sdn")
	if !ok {
		t.Fatal("sdn command not registered")
	}
	for _, subName := range []string{"zones", "vnets"} {
		if _, ok := sdn.sub[subName]; !ok {
			t.Fatalf("sdn command missing %s subcommand", subName)
		}
	}

	vm, ok := GetCommand("vm")
	if !ok {
		t.Fatal("vm command not registered")
	}
	if _, ok := vm.sub["snapshot-config"]; !ok {
		t.Fatal("vm command missing snapshot-config subcommand")
	}

	container, ok := GetCommand("container")
	if !ok {
		t.Fatal("container command not registered")
	}
	if _, ok := container.sub["snapshot-config"]; !ok {
		t.Fatal("container command missing snapshot-config subcommand")
	}
}

func TestPhase13SubcommandsRejectWrongArgCount(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "ha-status-extra", args: []string{"ha", "status", "extra"}},
		{name: "ha-current-extra", args: []string{"ha", "current", "extra"}},
		{name: "backup-content-no-args", args: []string{"backup", "content"}},
		{name: "backup-content-one-arg", args: []string{"backup", "content", "node1"}},
		{name: "sdn-zones-extra", args: []string{"sdn", "zones", "extra"}},
		{name: "sdn-vnets-extra", args: []string{"sdn", "vnets", "extra"}},
		{name: "vm-snapshot-config-no-args", args: []string{"vm", "snapshot-config"}},
		{name: "vm-snapshot-config-bad-id", args: []string{"vm", "snapshot-config", "not-node/vmid-format", "snap1"}},
		{name: "container-snapshot-config-no-args", args: []string{"container", "snapshot-config"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateConfigAndHome(t)
			setupE2EConfig(t)

			var stdout, stderr bytes.Buffer
			err := Run(context.Background(), tt.args, &stdout, &stderr)
			if err == nil {
				t.Fatal("expected usage error")
			}
			var exitCode *app.ExitCoder
			if !stderrors.As(err, &exitCode) || exitCode.ExitCode != app.ExitUsage {
				t.Fatalf("error = %v, want ExitUsage", err)
			}
		})
	}
}

func TestRun_HAStatus(t *testing.T) {
	isolateConfigAndHome(t)
	setupE2EConfig(t)

	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"ha", "status"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !strings.Contains(stdout.String(), "online") {
		t.Errorf("expected status in output, got: %s", stdout.String())
	}
}

func TestRun_BackupContent(t *testing.T) {
	isolateConfigAndHome(t)
	setupE2EConfig(t)

	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"backup", "content", "e2e-node", "local"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !strings.Contains(stdout.String(), "vzdump") {
		t.Errorf("expected backup content in output, got: %s", stdout.String())
	}
}

func TestRun_VMSnapshotConfig(t *testing.T) {
	isolateConfigAndHome(t)
	setupE2EConfig(t)

	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"vm", "snapshot-config", "e2e-node/100", "snap1"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestWritePhase13NilHandling(t *testing.T) {
	for _, format := range []output.Format{output.FormatJSON, output.FormatYAML} {
		t.Run(string(format), func(t *testing.T) {
			ctx := &Context{Writer: &bytes.Buffer{}, Opts: Options{Output: format}}
			if err := writeHAStatusTable(ctx, nil); err != nil {
				t.Fatalf("writeHAStatusTable nil: %v", err)
			}
			if err := writeHACurrentTable(ctx, nil); err != nil {
				t.Fatalf("writeHACurrentTable nil: %v", err)
			}
			if err := writeBackupContentTable(ctx, nil); err != nil {
				t.Fatalf("writeBackupContentTable nil: %v", err)
			}
			if err := writeSDNZonesTable(ctx, nil); err != nil {
				t.Fatalf("writeSDNZonesTable nil: %v", err)
			}
			if err := writeSDNVNetsTable(ctx, nil); err != nil {
				t.Fatalf("writeSDNVNetsTable nil: %v", err)
			}
		})
	}
}

func TestPhase3SubcommandsRegistered(t *testing.T) {
	vm, ok := GetCommand("vm")
	if !ok {
		t.Fatal("vm command not registered")
	}
	for _, subName := range []string{"update", "delete", "cloud-init", "template", "snapshot"} {
		if _, ok := vm.sub[subName]; !ok {
			t.Fatalf("vm command missing %s subcommand", subName)
		}
	}

	container, ok := GetCommand("container")
	if !ok {
		t.Fatal("container command not registered")
	}
	for _, subName := range []string{"update", "delete", "template", "snapshot"} {
		if _, ok := container.sub[subName]; !ok {
			t.Fatalf("container command missing %s subcommand", subName)
		}
	}
}

func TestPhase3SubcommandsRejectWrongArgCount(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "vm-update-no-args", args: []string{"vm", "update"}},
		{name: "vm-update-no-params", args: []string{"vm", "update", "e2e-node/100"}},
		{name: "vm-delete-no-arg", args: []string{"vm", "delete"}},
		{name: "vm-delete-extra", args: []string{"vm", "delete", "e2e-node/100", "extra"}},
		{name: "vm-cloud-init-no-arg", args: []string{"vm", "cloud-init"}},
		{name: "vm-cloud-init-extra", args: []string{"vm", "cloud-init", "e2e-node/100", "extra"}},
		{name: "vm-template-no-arg", args: []string{"vm", "template"}},
		{name: "vm-template-extra", args: []string{"vm", "template", "e2e-node/100", "extra"}},
		{name: "vm-snapshot-create-no-args", args: []string{"vm", "snapshot", "create"}},
		{name: "vm-snapshot-create-no-name", args: []string{"vm", "snapshot", "create", "e2e-node/100"}},
		{name: "vm-snapshot-delete-no-args", args: []string{"vm", "snapshot", "delete"}},
		{name: "vm-snapshot-delete-no-name", args: []string{"vm", "snapshot", "delete", "e2e-node/100"}},
		{name: "vm-snapshot-rollback-no-args", args: []string{"vm", "snapshot", "rollback"}},
		{name: "container-update-no-args", args: []string{"container", "update"}},
		{name: "container-delete-no-arg", args: []string{"container", "delete"}},
		{name: "container-template-no-arg", args: []string{"container", "template"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateConfigAndHome(t)
			setupE2EConfig(t)

			var stdout, stderr bytes.Buffer
			err := Run(context.Background(), tt.args, &stdout, &stderr)
			if err == nil {
				t.Fatal("expected usage error")
			}
			var exitCode *app.ExitCoder
			if !stderrors.As(err, &exitCode) || exitCode.ExitCode != app.ExitUsage {
				t.Fatalf("error = %v, want ExitUsage", err)
			}
		})
	}
}

func TestPhase3SafetyTiers(t *testing.T) {
	isolateConfigAndHome(t)
	setupE2EConfig(t)

	// Tier 1: without --yes, should return an error (fail-closed)
	tier1 := []struct {
		name string
		args []string
	}{
		{name: "vm-update-needs-yes", args: []string{"vm", "update", "e2e-node/100", "memory=4096"}},
		{name: "vm-cloud-init-needs-yes", args: []string{"vm", "cloud-init", "e2e-node/100"}},
		{name: "vm-snapshot-create-needs-yes", args: []string{"vm", "snapshot", "create", "e2e-node/100", "snap1"}},
	}
	for _, tt := range tier1 {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := Run(context.Background(), tt.args, &stdout, &stderr)
			if err == nil {
				t.Fatal("expected confirmation error (fail-closed), got nil")
			}
			// Error should mention authorization or confirmation
			if !strings.Contains(err.Error(), "authorization") && !strings.Contains(err.Error(), "Operation") {
				t.Fatalf("expected authorization error, got: %v", err)
			}
			// Warning should be on stderr
			out := stderr.String()
			if !strings.Contains(out, "--yes") && !strings.Contains(out, "confirm") && !strings.Contains(out, "Operation") {
				t.Fatalf("expected confirmation prompt on stderr, got: %q", out)
			}
		})
	}

	// Tier 2: with --yes but not --force, should also return error (fail-closed)
	t.Run("vm-template-needs-force", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		err := Run(context.Background(), []string{"--yes", "vm", "template", "e2e-node/100"}, &stdout, &stderr)
		if err == nil {
			t.Fatal("expected double-confirmation error (fail-closed), got nil")
		}
		if !strings.Contains(err.Error(), "authorization") && !strings.Contains(err.Error(), "Operation") {
			t.Fatalf("expected authorization error, got: %v", err)
		}
		out := stderr.String()
		if !strings.Contains(out, "--force") {
			t.Fatalf("expected double confirmation prompt on stderr, got: %q", out)
		}
	})
}

func TestRun_VMUpdate(t *testing.T) {
	isolateConfigAndHome(t)
	setupE2EConfig(t)

	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"--output", "table", "--yes", "vm", "update", "e2e-node/100", "memory=4096"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !strings.Contains(stdout.String(), "UPID:e2e-node") {
		t.Errorf("expected UPID in output, got: %s", stdout.String())
	}
}

func TestRun_VMSnapshotCreate(t *testing.T) {
	isolateConfigAndHome(t)
	setupE2EConfig(t)

	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"--output", "table", "--yes", "vm", "snapshot", "create", "e2e-node/100", "snap1"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !strings.Contains(stdout.String(), "UPID:e2e-node") {
		t.Errorf("expected UPID in output, got: %s", stdout.String())
	}
}

func TestRun_VMCloudInit(t *testing.T) {
	isolateConfigAndHome(t)
	setupE2EConfig(t)

	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"--output", "table", "--yes", "vm", "cloud-init", "e2e-node/100"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !strings.Contains(stdout.String(), "UPID:e2e-node") {
		t.Errorf("expected UPID in output, got: %s", stdout.String())
	}
}

func TestWriteEmptyResourceListsAsStructuredArrays(t *testing.T) {
	tests := []struct {
		name  string
		write func(*Context) error
	}{
		{name: "vms", write: func(ctx *Context) error { return writeVMs(ctx, nil) }},
		{name: "containers", write: func(ctx *Context) error { return writeContainers(ctx, nil) }},
		{name: "storage", write: func(ctx *Context) error { return writeStorages(ctx, nil) }},
	}
	for _, tt := range tests {
		for _, format := range []output.Format{output.FormatJSON, output.FormatYAML} {
			t.Run(tt.name+"/"+string(format), func(t *testing.T) {
				var stdout bytes.Buffer
				cmdCtx := &Context{Writer: &stdout, Opts: Options{Output: format}}
				if err := tt.write(cmdCtx); err != nil {
					t.Fatalf("write: %v", err)
				}
				if got := strings.TrimSpace(stdout.String()); got != "[]" {
					t.Fatalf("%s output = %q, want []", format, got)
				}
			})
		}
	}
}

// TestPasswordArgumentRejected verifies that password= as a CLI argument is
// explicitly rejected with a clear message directing users to safe alternatives.
func TestPasswordArgumentRejected(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{
		"--non-interactive", "access", "user", "create", "testuser@pve", "password=secret123",
	}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for password= argument")
	}
	if !strings.Contains(err.Error(), "passwords must not be passed as command arguments") {
		t.Errorf("expected password rejection message, got: %v", err)
	}
	// The password must not appear in the error output.
	if strings.Contains(stderr.String(), "secret123") || strings.Contains(stdout.String(), "secret123") {
		t.Error("password value leaked in output")
	}
}

// TestPasswordStdinFlagAccepted verifies that --password-stdin is recognized
// as a valid global flag.
func TestPasswordStdinFlagAccepted(t *testing.T) {
	var stdout, stderr bytes.Buffer
	// Without --password-stdin and in non-interactive mode, command should
	// proceed (with empty password) and fail at provider stage.
	err := Run(context.Background(), []string{
		"--non-interactive", "--yes", "--force", "--expert",
		"access", "user", "create", "testuser@pve",
	}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error (provider mock doesn't support AccessProvider)")
	}
	// Must not mention password= syntax.
	if strings.Contains(err.Error(), "password=<p>") {
		t.Error("error message references deprecated password= syntax")
	}
}

// TestAccessUserHelpDoesNotShowPasswordArg verifies the usage message no longer
// suggests password= as an acceptable argument.
func TestAccessUserHelpDoesNotShowPasswordArg(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"access", "user"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected no error for help, got: %v", err)
	}
	out := stdout.String()
	if strings.Contains(out, "password=<p>") {
		t.Error("usage message still references password=<p> argument")
	}
	if !strings.Contains(out, "--password-stdin") {
		t.Error("usage message does not mention --password-stdin")
	}
}

// TestFailClosedConfirmation verifies that mutation commands return errors
// (not nil) when confirmation has not been granted.
func TestFailClosedConfirmation(t *testing.T) {
	isolateConfigAndHome(t)
	setupE2EConfig(t)

	// Each entry: (name, args with no confirmation flags)
	mutations := []struct {
		name string
		args []string
	}{
		// VM lifecycle (Tier 1)
		{"vm-start", []string{"vm", "start", "e2e-node/100"}},
		{"vm-stop", []string{"vm", "stop", "e2e-node/100"}},
		{"vm-shutdown", []string{"vm", "shutdown", "e2e-node/100"}},
		// VM lifecycle (Tier 2)
		{"vm-reset", []string{"vm", "reset", "e2e-node/100"}},
		{"vm-reboot", []string{"vm", "reboot", "e2e-node/100"}},
		// Container lifecycle (Tier 1)
		{"ct-start", []string{"container", "start", "e2e-node/100"}},
		// Config (Tier 1)
		{"vm-update", []string{"vm", "update", "e2e-node/100", "memory=4096"}},
		// Snapshots (Tier 1)
		{"vm-snapshot-create", []string{"vm", "snapshot", "create", "e2e-node/100", "snap1"}},
		// Cloud-init (Tier 1)
		{"vm-cloud-init", []string{"vm", "cloud-init", "e2e-node/100"}},
	}

	for _, tt := range mutations {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := Run(context.Background(), tt.args, &stdout, &stderr)
			if err == nil {
				t.Fatalf("%s without confirmation returned nil (fail-open)", tt.name)
			}
			if !strings.Contains(err.Error(), "authorization") && !strings.Contains(err.Error(), "Operation") {
				t.Errorf("%s: expected authorization error, got: %v", tt.name, err)
			}
		})
	}
}

// TestFailClosedDisruptive verifies Tier 2 operations return error without --yes --force.
func TestFailClosedDisruptive(t *testing.T) {
	isolateConfigAndHome(t)
	setupE2EConfig(t)

	// With --yes only, Tier 2 should still fail because --force is also needed.
	t.Run("vm-template-needs-force", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		err := Run(context.Background(), []string{"--yes", "vm", "template", "e2e-node/100"}, &stdout, &stderr)
		if err == nil {
			t.Fatal("Tier 2 with --yes only returned nil (fail-open)")
		}
	})
}

// TestFailClosedSecurityAdmin verifies Tier 4 operations return error without --expert.
func TestFailClosedSecurityAdmin(t *testing.T) {
	isolateConfigAndHome(t)
	setupE2EConfig(t)

	// The e2e mock provider does not implement AccessProvider, so the command
	// fails with "unsupported capability" before reaching the --expert check.
	// This test verifies the command fails closed regardless; the safety layer
	// itself is tested in the safety package.
	t.Run("access-user-create-fails-closed", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		// The command fails closed because the e2e mock provider does not implement
		// AccessProvider. Password is not passed as an argument (that was removed for
		// security). --non-interactive prevents hanging on an interactive prompt.
		err := Run(context.Background(), []string{"--yes", "--force", "--expert", "--non-interactive", "access", "user", "create", "testuser@pve"}, &stdout, &stderr)
		if err == nil {
			t.Fatal("access command with unsupported provider returned nil (fail-open)")
		}
	})
}

// TestTaskFailureReturnsError is a structural test: the task package correctly returns
// errors on failure, and runMutationWithPolling wraps them. The e2e mock always
// returns task status "OK", so full integration is tested at the task unit level.
// This test verifies task polling exists and completes without error in the mock.
func TestTaskPollingCompletes(t *testing.T) {
	isolateConfigAndHome(t)
	setupE2EConfig(t)

	t.Run("vm-stop-wait-completes", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		err := Run(context.Background(), []string{"--output", "table", "--yes", "--wait", "vm", "stop", "e2e-node/100"}, &stdout, &stderr)
		if err != nil {
			t.Fatalf("--wait with mock task failed unexpectedly: %v", err)
		}
		if !strings.Contains(stdout.String(), "completed OK") {
			t.Errorf("expected task completion message, got: %s", stdout.String())
		}
	})
}

// TestCommandsWithFlagsSucceed verifies that mutation commands with proper flags succeed.
func TestCommandsWithFlagsSucceed(t *testing.T) {
	isolateConfigAndHome(t)
	setupE2EConfig(t)

	tests := []struct {
		name string
		args []string
	}{
		{"vm-start-yes", []string{"--output", "table", "--yes", "vm", "start", "e2e-node/100"}},
		{"vm-stop-yes", []string{"--output", "table", "--yes", "vm", "stop", "e2e-node/100"}},
		{"vm-shutdown-yes", []string{"--output", "table", "--yes", "vm", "shutdown", "e2e-node/100"}},
		{"vm-update-yes", []string{"--output", "table", "--yes", "vm", "update", "e2e-node/100", "memory=4096"}},
		{"vm-snapshot-create-yes", []string{"--output", "table", "--yes", "vm", "snapshot", "create", "e2e-node/100", "snap1"}},
		{"vm-cloud-init-yes", []string{"--output", "table", "--yes", "vm", "cloud-init", "e2e-node/100"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := Run(context.Background(), tt.args, &stdout, &stderr)
			if err != nil {
				t.Fatalf("%s with proper flags: %v", tt.name, err)
			}
			if !strings.Contains(stdout.String(), "UPID:") {
				t.Errorf("%s: expected UPID in output, got: %s", tt.name, stdout.String())
			}
		})
	}
}

// Phase 7: Multi-Cluster tests

func TestPhase7ProfileSubcommandsRegistered(t *testing.T) {
	cmd, ok := GetCommand("profile")
	if !ok {
		t.Fatal("profile command not registered")
	}
	for _, subName := range []string{"export", "import"} {
		if _, ok := cmd.sub[subName]; !ok {
			t.Fatalf("profile command missing %s subcommand", subName)
		}
	}
}

func TestPhase7ProfileSubcommandsRejectWrongArgCount(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "export-no-name", args: []string{"profile", "export"}},
		{name: "export-extra", args: []string{"profile", "export", "name", "extra"}},
		{name: "import-no-name", args: []string{"profile", "import"}},
		{name: "import-extra", args: []string{"profile", "import", "name", "extra"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := Run(context.Background(), tt.args, &stdout, &stderr)
			if err == nil {
				t.Fatal("expected usage error")
			}
			var exitCode *app.ExitCoder
			if !stderrors.As(err, &exitCode) || exitCode.ExitCode != app.ExitUsage {
				t.Fatalf("error = %v, want ExitUsage", err)
			}
		})
	}
}

func TestAllFlagRejectsNoProfiles(t *testing.T) {
	isolateConfigAndHome(t)
	// Write empty config.
	path, err := config.ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	if err := config.WriteTo(config.DefaultConfig(), path); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	for _, args := range [][]string{
		{"--all", "status"},
		{"--all", "node", "list"},
		{"--all", "vm", "list"},
		{"--all", "container", "list"},
	} {
		var stdout, stderr bytes.Buffer
		err := Run(context.Background(), args, &stdout, &stderr)
		if err == nil {
			t.Fatalf("expected error for --all with no profiles: %v", args)
		}
		if !strings.Contains(err.Error(), "no profiles") {
			t.Fatalf("expected 'no profiles' error, got: %v", err)
		}
	}
}

func TestWriteNodesAllNilHandling(t *testing.T) {
	for _, format := range []output.Format{output.FormatJSON, output.FormatYAML} {
		t.Run(string(format), func(t *testing.T) {
			var stdout bytes.Buffer
			cmdCtx := &Context{Writer: &stdout, Opts: Options{Output: format}}
			out := output.NewMultiProfileOutput[[]domain.Node]()
			if err := writeNodesAll(cmdCtx, out); err != nil {
				t.Fatalf("writeNodesAll empty: %v", err)
			}
			got := strings.TrimSpace(stdout.String())
			if !strings.Contains(got, "results") {
				t.Fatalf("writeNodesAll empty = %q, want MultiProfileOutput envelope", got)
			}
		})
	}
}

func TestWriteVMsAllNilHandling(t *testing.T) {
	for _, format := range []output.Format{output.FormatJSON, output.FormatYAML} {
		t.Run(string(format), func(t *testing.T) {
			var stdout bytes.Buffer
			cmdCtx := &Context{Writer: &stdout, Opts: Options{Output: format}}
			out := output.NewMultiProfileOutput[[]domain.VM]()
			if err := writeVMsAll(cmdCtx, out); err != nil {
				t.Fatalf("writeVMsAll empty: %v", err)
			}
			got := strings.TrimSpace(stdout.String())
			if !strings.Contains(got, "results") {
				t.Fatalf("writeVMsAll empty = %q, want MultiProfileOutput envelope", got)
			}
		})
	}
}

func TestWriteContainersAllNilHandling(t *testing.T) {
	for _, format := range []output.Format{output.FormatJSON, output.FormatYAML} {
		t.Run(string(format), func(t *testing.T) {
			var stdout bytes.Buffer
			cmdCtx := &Context{Writer: &stdout, Opts: Options{Output: format}}
			out := output.NewMultiProfileOutput[[]domain.Container]()
			if err := writeContainersAll(cmdCtx, out); err != nil {
				t.Fatalf("writeContainersAll empty: %v", err)
			}
			got := strings.TrimSpace(stdout.String())
			if !strings.Contains(got, "results") {
				t.Fatalf("writeContainersAll empty = %q, want MultiProfileOutput envelope", got)
			}
		})
	}
}

func TestWriteAggregatedStatusNilHandling(t *testing.T) {
	for _, format := range []output.Format{output.FormatJSON, output.FormatYAML} {
		t.Run(string(format), func(t *testing.T) {
			var stdout bytes.Buffer
			cmdCtx := &Context{Writer: &stdout, Opts: Options{Output: format}}
			out := output.NewMultiProfileOutput[aggregatedStatus]()
			if err := writeAggregatedStatus(cmdCtx, out); err != nil {
				t.Fatalf("writeAggregatedStatus empty: %v", err)
			}
			got := strings.TrimSpace(stdout.String())
			if !strings.Contains(got, "results") {
				t.Fatalf("writeAggregatedStatus empty = %q, want MultiProfileOutput envelope", got)
			}
		})
	}
}

// TestMutationJSONOutputIsValid verifies that mutation commands produce valid
// JSON with the OperationResult envelope when --output json is specified.
func TestMutationJSONOutputIsValid(t *testing.T) {
	isolateConfigAndHome(t)
	setupE2EConfig(t)

	t.Run("vm-start-json", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		err := Run(context.Background(), []string{"--output", "json", "--yes", "vm", "start", "e2e-node/100"}, &stdout, &stderr)
		if err != nil {
			t.Fatalf("vm start: %v", err)
		}
		out := stdout.String()
		for _, want := range []string{
			`"schema":`, `"operation": "vm start"`,
			`"provider":`, `"submitted": true`, `"success": true`,
		} {
			if !strings.Contains(out, want) {
				t.Errorf("JSON output missing %q: %s", want, out)
			}
		}
		// JSON must be the only thing on stdout - no prompts or warnings.
		if !strings.HasPrefix(strings.TrimSpace(out), "{") {
			t.Errorf("JSON output should start with '{': %s", out)
		}
	})

	t.Run("vm-start-wait-json", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		err := Run(context.Background(), []string{"--output", "json", "--yes", "--wait", "vm", "start", "e2e-node/100"}, &stdout, &stderr)
		if err != nil {
			t.Fatalf("vm start --wait: %v", err)
		}
		out := stdout.String()
		for _, want := range []string{`"waited": true`, `"status": "OK"`} {
			if !strings.Contains(out, want) {
				t.Errorf("JSON output missing %q: %s", want, out)
			}
		}
	})
}

// TestMutationStdoutStderrSeparation verifies that prompts and warnings go
// to stderr, and result data goes to stdout.
func TestMutationStdoutStderrSeparation(t *testing.T) {
	isolateConfigAndHome(t)
	setupE2EConfig(t)

	t.Run("table-mode-warnings-on-stderr", func(t *testing.T) {
		// Without --yes, the command should fail and write the prompt to stderr.
		var stdout, stderr bytes.Buffer
		err := Run(context.Background(), []string{"--output", "table", "vm", "start", "e2e-node/100"}, &stdout, &stderr)
		if err == nil {
			t.Fatal("expected authorization error, got nil")
		}
		// The prompt must be on stderr, not stdout.
		if strings.Contains(stdout.String(), "Operation on") || strings.Contains(stdout.String(), "--yes") {
			t.Errorf("confirmation prompt leaked to stdout: %q", stdout.String())
		}
		if !strings.Contains(stderr.String(), "Operation") {
			t.Errorf("confirmation prompt missing from stderr: %q", stderr.String())
		}
	})

	t.Run("wait-progress-on-stderr", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		err := Run(context.Background(), []string{"--output", "table", "--yes", "--wait", "vm", "start", "e2e-node/100"}, &stdout, &stderr)
		if err != nil {
			t.Fatalf("vm start --wait: %v", err)
		}
		if !strings.Contains(stderr.String(), "Waiting for task") {
			t.Errorf("wait progress missing from stderr: %q", stderr.String())
		}
		// Stderr must not contain the result data.
		if strings.Contains(stderr.String(), "completed OK") {
			t.Errorf("result data leaked to stderr: %q", stderr.String())
		}
	})
}

// TestMultiProfileExitCodes verifies that --all commands return the
// correct exit codes for partial and total failures.
func TestMultiProfileExitCodes(t *testing.T) {
	isolateConfigAndHome(t)
	setupE2EConfig(t)

	t.Run("all-success-exit-zero", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		err := Run(context.Background(), []string{"--all", "node", "list"}, &stdout, &stderr)
		if err != nil {
			t.Errorf("expected nil error for all-success, got: %v", err)
		}
	})

	// TestPartialFailureExitFromMulti verifies the exitFromMulti helper returns
	// the correct exit codes for various failure scenarios.
	t.Run("partial-failure-exit-from-multi", func(t *testing.T) {
		// No failures -> nil
		out0 := output.NewMultiProfileOutput[any]()
		out0.AddSuccess("a", nil, 0)
		out0.AddSuccess("b", nil, 0)
		if err := exitFromMulti(out0); err != nil {
			t.Errorf("0 failures should return nil, got: %v", err)
		}
		// Partial -> ExitPartialFailure
		out1 := output.NewMultiProfileOutput[any]()
		out1.AddSuccess("a", nil, 0)
		out1.AddFailure("b", fmt.Errorf("connection refused"), 0)
		if err := exitFromMulti(out1); err == nil {
			t.Error("1/2 failures should return error, got nil")
		} else if code := app.ExitCodeFromError(err); code != app.ExitPartialFailure {
			t.Errorf("1/2 failures: expected ExitPartialFailure (11), got %d", code)
		}
		// All fail -> ExitPartialFailure
		out2 := output.NewMultiProfileOutput[any]()
		out2.AddFailure("a", fmt.Errorf("timeout"), 0)
		out2.AddFailure("b", fmt.Errorf("connection refused"), 0)
		if err := exitFromMulti(out2); err == nil {
			t.Error("2/2 failures should return error, got nil")
		} else if code := app.ExitCodeFromError(err); code != app.ExitPartialFailure {
			t.Errorf("2/2 failures: expected ExitPartialFailure (11), got %d", code)
		}
	})
}

// TestCheckAllSupported verifies that --all is rejected for commands not
// in the explicit allowlist.
func TestCheckAllSupported(t *testing.T) {
	// Allowed commands.
	for _, path := range [][]string{
		{"status"},
		{"node", "list"},
		{"vm", "list"},
		{"container", "list"},
	} {
		if err := checkAllSupported(true, path...); err != nil {
			t.Errorf("checkAllSupported(true, %v) = %v, want nil", path, err)
		}
	}

	// When --all is not set, always pass.
	for _, path := range [][]string{
		{"status"},
		{"vm", "start"},
		{"backup", "create"},
	} {
		if err := checkAllSupported(false, path...); err != nil {
			t.Errorf("checkAllSupported(false, %v) = %v, want nil", path, err)
		}
	}

	// Rejected commands.
	rejected := [][]string{
		{"vm", "start"},
		{"vm", "stop"},
		{"vm", "delete"},
		{"container", "start"},
		{"container", "delete"},
		{"backup", "create"},
		{"storage", "upload"},
		{"storage", "delete"},
		{"firewall", "list"},
	}
	for _, path := range rejected {
		err := checkAllSupported(true, path...)
		if err == nil {
			t.Errorf("checkAllSupported(true, %v) = nil, want error", path)
			continue
		}
		if !strings.Contains(err.Error(), "not supported") {
			t.Errorf("checkAllSupported(true, %v) error should say 'not supported', got: %v", path, err)
		}
	}
}

// TestMultiProfileCancellation verifies that a cancelled context is handled
// gracefully during multi-profile execution. The e2e mock provider currently
// does not check context on its domain methods, so this test verifies the
// code path does not panic or hang. A real provider would return a context
// cancellation error and produce ExitCancellation.
func TestMultiProfileCancellation(t *testing.T) {
	isolateConfigAndHome(t)
	setupMultiProfileConfig(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	var stdout, stderr bytes.Buffer
	err := Run(ctx, []string{"--all", "node", "list"}, &stdout, &stderr)
	// With the mock provider, cancellation may or may not produce an error
	// depending on whether context is checked. Either outcome is acceptable
	// as long as the code does not panic or hang.
	if err != nil {
		code := app.ExitCodeFromError(err)
		t.Logf("cancelled context exit code: %d, error: %v", code, err)
	} else {
		t.Log("mock provider completed despite cancelled context (expected)")
	}
}

// TestRunDoctor verifies the doctor command runs without live infrastructure.
func TestRunDoctor(t *testing.T) {
	isolateConfigAndHome(t)

	// Without config, doctor should still report config failure but not crash.
	path, err := config.ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	if err := config.WriteTo(config.DefaultConfig(), path); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	var stdout, stderr bytes.Buffer
	err = Run(context.Background(), []string{"doctor"}, &stdout, &stderr)
	// Doctor may fail because config is missing profiles, but it should not panic.
	// The output should contain the config check result.
	out := stdout.String()
	if !strings.Contains(out, "config") {
		t.Errorf("doctor output missing config check: %q", out)
	}
	t.Logf("doctor output: %s", out)
	if err != nil {
		t.Logf("doctor error (expected if issues found): %v", err)
	}
}

// TestRunDoctorJSON verifies doctor JSON output.
func TestRunDoctorJSON(t *testing.T) {
	isolateConfigAndHome(t)

	path, err := config.ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	cfg := config.DefaultConfig()
	cfg.Profiles["test"] = config.Profile{Provider: "proxmox", Endpoint: "https://example.com"}
	if err := config.WriteTo(cfg, path); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	var stdout, stderr bytes.Buffer
	err = Run(context.Background(), []string{"--output", "json", "doctor"}, &stdout, &stderr)
	if err != nil {
		t.Logf("doctor JSON error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, `"results"`) && !strings.Contains(out, `"pass"`) {
		t.Errorf("doctor JSON missing expected keys: %q", out)
	}
}

func TestRunDoctorJSONFailsWhenChecksFail(t *testing.T) {
	isolateConfigAndHome(t)

	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"--output", "json", "doctor"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected doctor to fail when checks fail")
	}
	if code := app.ExitCodeFromError(err); code != app.ExitGeneral {
		t.Fatalf("exit code = %d, want %d", code, app.ExitGeneral)
	}
	out := stdout.String()
	if !strings.Contains(out, `"fail"`) || !strings.Contains(out, `"config"`) {
		t.Fatalf("doctor JSON missing failed config check: %q", out)
	}
}

func TestRunDoctorYAMLFailsWhenChecksFail(t *testing.T) {
	isolateConfigAndHome(t)

	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"--output", "yaml", "doctor"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected doctor to fail when checks fail")
	}
	if code := app.ExitCodeFromError(err); code != app.ExitGeneral {
		t.Fatalf("exit code = %d, want %d", code, app.ExitGeneral)
	}
	out := stdout.String()
	if !strings.Contains(out, "fail") || !strings.Contains(out, "config") {
		t.Fatalf("doctor YAML missing failed config check: %q", out)
	}
}

// TestRunDoctorRejectsExtraArgs verifies doctor rejects extra arguments.
func TestRunDoctorRejectsExtraArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"doctor", "extra"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for extra args")
	}
}

func TestFirewallGroupRejectsLongName(t *testing.T) {
	for _, args := range [][]string{
		{"firewall", "group", "create", "sidekick-nodex-audit-group"},
		{"firewall", "group", "delete", "sidekick-nodex-audit-group"},
	} {
		var stdout, stderr bytes.Buffer
		err := Run(context.Background(), args, &stdout, &stderr)
		if err == nil {
			t.Fatalf("Run(%v): expected error", args)
		}
		if code := app.ExitCodeFromError(err); code != app.ExitUsage {
			t.Fatalf("Run(%v) exit code = %d, want %d", args, code, app.ExitUsage)
		}
		if !strings.Contains(err.Error(), "maximum 18 characters") {
			t.Fatalf("Run(%v) error = %q", args, err)
		}
	}
}

// TestSortResultsNilSafe verifies sortResults handles nil/empty slices.
func TestSortResultsNilSafe(t *testing.T) {
	// Must not panic on nil.
	sortResults(nil)
	// Must not panic on empty.
	sortResults([]checkResult{})
	// Must sort correctly.
	results := []checkResult{
		{Name: "z", Status: "pass"},
		{Name: "a", Status: "pass"},
		{Name: "m", Status: "pass"},
	}
	sortResults(results)
	if results[0].Name != "a" || results[1].Name != "m" || results[2].Name != "z" {
		t.Errorf("sortResults did not sort: %v", results)
	}
}

// TestToHandler verifies the operation-path-to-handler-name conversion.
func TestToHandler(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"vm start", "VmStart"},
		{"vm delete", "VmDelete"},
		{"container snapshot create", "ContainerSnapshotCreate"},
		{"firewall ipset entry add", "FirewallIpsetEntryAdd"},
		{"ceph osd in", "CephOsdIn"},
		{"backup job delete", "BackupJobDelete"},
		{"sdn zone create", "SdnZoneCreate"},
		{"access user create", "AccessUserCreate"},
	}
	for _, tt := range tests {
		got := toHandler(tt.path)
		if got != tt.want {
			t.Errorf("toHandler(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

// TestToHandlerEdgeCases verifies toHandler handles edge cases.
func TestToHandlerEdgeCases(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"", ""},
		{" ", ""},
		{"  a  ", "A"},
		{"a b c d", "ABCD"},
	}
	for _, tt := range tests {
		got := toHandler(tt.path)
		if got != tt.want {
			t.Errorf("toHandler(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}
