package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/geoffmcc/nodex/internal/config"
	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/provider"
)

type containerOSProbe struct {
	*e2eMockProvider
	read func(context.Context, string, int) (map[string]interface{}, error)
}

func (p *containerOSProbe) ContainerConfig(ctx context.Context, node string, vmid int) (map[string]interface{}, error) {
	return p.read(ctx, node, vmid)
}

func TestContainerOSUsesOnlyAuthoritativeConfigForReturnedTargets(t *testing.T) {
	var calls []string
	p := &containerOSProbe{read: func(ctx context.Context, node string, vmid int) (map[string]interface{}, error) {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > time.Second {
			t.Fatal("unbounded OS enrichment")
		}
		calls = append(calls, node)
		switch vmid {
		case 200:
			return map[string]interface{}{"ostype": "debian"}, nil
		case 201:
			return nil, errors.New("permission denied")
		default:
			return map[string]interface{}{"ostype": 42}, nil
		}
	}}
	cts := []domain.Container{{ID: "node-a/200"}, {ID: "node-b/201"}, {ID: "node-c/202"}, {ID: "node-d/203", Name: "known", CPU: 1, Memory: 1, Disk: 1, OS: "ubuntu"}, {ID: "invalid"}}
	enrichContainerConfig(t.Context(), p, cts, false)
	if strings.Join(calls, ",") != "node-a,node-b,node-c" || cts[0].OS != "debian" || cts[1].OS != "" || cts[2].OS != "" || cts[3].OS != "ubuntu" {
		t.Fatal(calls, cts)
	}
}

func TestContainerConfigEnrichmentFillsRestoredInventoryGaps(t *testing.T) {
	calls := 0
	p := &containerOSProbe{read: func(context.Context, string, int) (map[string]interface{}, error) {
		calls++
		return map[string]interface{}{
			"hostname": "nodex-restore-source",
			"cores":    1,
			"memory":   512,
			"rootfs":   "local-lvm:vm-9993-disk-0,size=4G",
			"ostype":   "ubuntu",
		}, nil
	}}
	containers := []domain.Container{{ID: "pve-nodex-lab/9993", Name: "CT9993", Status: "unknown", CPU: 4, Memory: 512 * 1024 * 1024, Disk: 4 * 1024 * 1024 * 1024, OS: "debian"}}
	enrichContainerConfig(t.Context(), p, containers, true)
	got := containers[0]
	if calls != 1 || got.Name != "nodex-restore-source" || got.CPU != 1 || got.Memory != 512*1024*1024 || got.Disk != 4*1024*1024*1024 || got.OS != "ubuntu" {
		t.Fatalf("enriched container = %+v; config reads = %d", got, calls)
	}
}

func TestParseContainerRootfsSize(t *testing.T) {
	tests := []struct {
		value string
		want  int64
	}{
		{"local-lvm:vm-100-disk-0,size=4G", 4 * 1024 * 1024 * 1024},
		{"local:100/vm-100-disk-0.raw,size=512M", 512 * 1024 * 1024},
		{"local-lvm:vm-100-disk-0,size=2048K", 2 * 1024 * 1024},
		{"local-lvm:vm-100-disk-0", 0},
		{"local-lvm:vm-100-disk-0,size=invalid", 0},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			if got := parseContainerRootfsSize(tt.value); got != tt.want {
				t.Fatalf("parseContainerRootfsSize(%q) = %d, want %d", tt.value, got, tt.want)
			}
		})
	}
}

func TestContainerOSEnrichmentHonorsCancellationAndJoinsReads(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	calls := 0
	p := &containerOSProbe{read: func(ctx context.Context, _ string, _ int) (map[string]interface{}, error) {
		calls++
		cancel()
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	cts := []domain.Container{{ID: "node/200"}, {ID: "node/201"}}
	enrichContainerConfig(ctx, p, cts, false)
	if calls != 1 || cts[0].OS != "" || cts[1].OS != "" {
		t.Fatal(calls, cts)
	}
}

const containerOSFixtureName = "nodex-container-os-fixture"

func init() {
	provider.Register(containerOSFixtureName, func() domain.Provider {
		return &containerOSProbe{e2eMockProvider: &e2eMockProvider{}, read: func(context.Context, string, int) (map[string]interface{}, error) {
			return map[string]interface{}{"ostype": "debian"}, nil
		}}
	})
}

func TestContainerOSVisibleThroughCLIAndAgent(t *testing.T) {
	isolateConfigAndHome(t)
	setupE2EConfig(t)
	cfg, err := config.Read()
	if err != nil {
		t.Fatal(err)
	}
	p := cfg.Profiles["e2e"]
	p.Provider = containerOSFixtureName
	cfg.Profiles["e2e"] = p
	if err := config.Write(cfg); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"--output", "json", "container", "list", "--limit", "1"},
		{"--output", "yaml", "container", "list"},
		{"--output", "table", "container", "list"},
		{"--output", "json", "container", "show", "e2e-node/200"},
		{"--agent", "--profile", "e2e", "container", "list"},
	} {
		var out, diag bytes.Buffer
		if err := Run(t.Context(), args, &out, &diag); err != nil {
			t.Fatalf("%v: %v %s", args, err, diag.String())
		}
		if !strings.Contains(out.String(), "debian") {
			t.Fatalf("%v: %s", args, out.String())
		}
		if args[0] == "--agent" {
			r := decodeAgentResult(t, out.Bytes())
			var cts []domain.Container
			if err := json.Unmarshal(r.Data, &cts); err != nil || len(cts) != 1 || cts[0].OS != "debian" {
				t.Fatal(err, string(r.Data))
			}
		}
	}
}
