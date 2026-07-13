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
