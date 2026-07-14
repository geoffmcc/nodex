package cli

import (
	"bytes"
	"context"
	stderrors "errors"
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
		err := Run(context.Background(), []string{"--yes", "--force", "--expert", "access", "user", "create", "testuser@pve", "password=test"}, &stdout, &stderr)
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
			if err := writeNodesAll(cmdCtx, nil); err != nil {
				t.Fatalf("writeNodesAll nil: %v", err)
			}
			if got := strings.TrimSpace(stdout.String()); got != "[]" {
				t.Fatalf("writeNodesAll nil = %q, want []", got)
			}
		})
	}
}

func TestWriteVMsAllNilHandling(t *testing.T) {
	for _, format := range []output.Format{output.FormatJSON, output.FormatYAML} {
		t.Run(string(format), func(t *testing.T) {
			var stdout bytes.Buffer
			cmdCtx := &Context{Writer: &stdout, Opts: Options{Output: format}}
			if err := writeVMsAll(cmdCtx, nil); err != nil {
				t.Fatalf("writeVMsAll nil: %v", err)
			}
			if got := strings.TrimSpace(stdout.String()); got != "[]" {
				t.Fatalf("writeVMsAll nil = %q, want []", got)
			}
		})
	}
}

func TestWriteContainersAllNilHandling(t *testing.T) {
	for _, format := range []output.Format{output.FormatJSON, output.FormatYAML} {
		t.Run(string(format), func(t *testing.T) {
			var stdout bytes.Buffer
			cmdCtx := &Context{Writer: &stdout, Opts: Options{Output: format}}
			if err := writeContainersAll(cmdCtx, nil); err != nil {
				t.Fatalf("writeContainersAll nil: %v", err)
			}
			if got := strings.TrimSpace(stdout.String()); got != "[]" {
				t.Fatalf("writeContainersAll nil = %q, want []", got)
			}
		})
	}
}

func TestWriteAggregatedStatusNilHandling(t *testing.T) {
	for _, format := range []output.Format{output.FormatJSON, output.FormatYAML} {
		t.Run(string(format), func(t *testing.T) {
			var stdout bytes.Buffer
			cmdCtx := &Context{Writer: &stdout, Opts: Options{Output: format}}
			if err := writeAggregatedStatus(cmdCtx, nil); err != nil {
				t.Fatalf("writeAggregatedStatus nil: %v", err)
			}
			if got := strings.TrimSpace(stdout.String()); got != "[]" && got != "null" {
				t.Fatalf("writeAggregatedStatus nil = %q, want [] or null", got)
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

	// TestPartialFailureAggregate verifies the aggregateError helper function
	// returns the correct exit codes for various failure scenarios.
	t.Run("partial-failure-aggregate", func(t *testing.T) {
		names := []string{"a", "b", "c"}
		// No failures -> nil
		if err := aggregateError(names, 0); err != nil {
			t.Errorf("0 failures should return nil, got: %v", err)
		}
		// Partial -> ExitPartialFailure
		if err := aggregateError(names, 1); err == nil {
			t.Error("1/3 failures should return error, got nil")
		} else if code := app.ExitCodeFromError(err); code != app.ExitPartialFailure {
			t.Errorf("1/3 failures: expected ExitPartialFailure (11), got %d", code)
		}
		// All fail -> ExitPartialFailure
		if err := aggregateError(names, 3); err == nil {
			t.Error("3/3 failures should return error, got nil")
		} else if code := app.ExitCodeFromError(err); code != app.ExitPartialFailure {
			t.Errorf("3/3 failures: expected ExitPartialFailure (11), got %d", code)
		}
	})
}
