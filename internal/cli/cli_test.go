package cli

import (
	"bytes"
	"context"
	stderrors "errors"
	"strings"
	"testing"
	"time"

	"github.com/geoffmcc/nodex/internal/app"
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

func TestWriteEmptyResourceListsAsJSONArrays(t *testing.T) {
	tests := []struct {
		name  string
		write func(*Context) error
	}{
		{name: "vms", write: func(ctx *Context) error { return writeVMs(ctx, nil) }},
		{name: "containers", write: func(ctx *Context) error { return writeContainers(ctx, nil) }},
		{name: "storage", write: func(ctx *Context) error { return writeStorages(ctx, nil) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			cmdCtx := &Context{Writer: &stdout, Opts: Options{Output: output.FormatJSON}}
			if err := tt.write(cmdCtx); err != nil {
				t.Fatalf("write: %v", err)
			}
			if got := strings.TrimSpace(stdout.String()); got != "[]" {
				t.Fatalf("JSON output = %q, want []", got)
			}
		})
	}
}
