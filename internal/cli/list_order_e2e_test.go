package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/geoffmcc/nodex/internal/config"
	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/provider"
)

// unorderedGuestProvider stands in for Proxmox's /cluster/resources, which
// documents no ordering guarantee and returns guests in an order that varies
// between calls. Every Containers/VMs call advances to the next rotation, which
// is what made --limit N a different subset on every invocation.
type unorderedGuestProvider struct {
	bareProvider
}

// unorderedGuestCall counts listing calls across the whole test binary. Each Run
// constructs a fresh provider, so the rotation cursor cannot live on the
// instance: what matters is that consecutive calls answer in different orders,
// which is what the real endpoint does.
var unorderedGuestCall int

const unorderedGuestProviderName = "nodex-unordered-guests"

func init() {
	provider.Register(unorderedGuestProviderName, func() domain.Provider { return &unorderedGuestProvider{} })
}

func (p *unorderedGuestProvider) Name() string { return unorderedGuestProviderName }

func (p *unorderedGuestProvider) Capabilities() []domain.Capability {
	return []domain.Capability{
		domain.CapabilityNodes, domain.CapabilityVMs,
		domain.CapabilityContainers, domain.CapabilityStorage,
	}
}

func (p *unorderedGuestProvider) current() []string {
	order := unorderedGuestRotations[unorderedGuestCall%len(unorderedGuestRotations)]
	unorderedGuestCall++
	return order
}

func (p *unorderedGuestProvider) Connect(_ context.Context, _ string, _ *domain.Credentials) error {
	return nil
}

func (p *unorderedGuestProvider) Containers(context.Context) ([]domain.Container, error) {
	order := p.current()
	containers := make([]domain.Container, 0, len(order))
	for _, id := range order {
		containers = append(containers, domain.Container{ID: id, Name: id, Node: "proxmox"})
	}
	return containers, nil
}

func (p *unorderedGuestProvider) VMs(context.Context) ([]domain.VM, error) {
	order := p.current()
	vms := make([]domain.VM, 0, len(order))
	for _, id := range order {
		vms = append(vms, domain.VM{ID: id, Name: id, Node: "proxmox"})
	}
	return vms, nil
}

// The listing interfaces require the per-guest config read alongside the
// listing; these satisfy the assertion so the command reaches the handler.
func (p *unorderedGuestProvider) VMConfig(context.Context, string, int) (map[string]interface{}, error) {
	return map[string]interface{}{}, nil
}

func (p *unorderedGuestProvider) ContainerConfig(context.Context, string, int) (map[string]interface{}, error) {
	return map[string]interface{}{}, nil
}

func (p *unorderedGuestProvider) StorageContent(context.Context, string, string) ([]domain.StorageContentItem, error) {
	return nil, nil
}

func (p *unorderedGuestProvider) Nodes(context.Context) ([]domain.Node, error) {
	return []domain.Node{{Name: "pve-b"}, {Name: "pve-a"}, {Name: "pve-c"}}, nil
}

func (p *unorderedGuestProvider) Storage(context.Context) ([]domain.Storage, error) {
	return []domain.Storage{{Name: "local", ID: "storage/proxmox/local"}, {Name: "backup", ID: "storage/proxmox/backup"}}, nil
}

// setupUnorderedGuestConfig registers the rotating provider and points the
// default profile at it.
func setupUnorderedGuestConfig(t *testing.T) {
	t.Helper()
	t.Setenv("NODEX_UNORDERED_TOKEN", "unordered-token")
	cfg := config.DefaultConfig()
	cfg.CurrentProfile = "unordered"
	cfg.Profiles["unordered"] = config.Profile{
		Provider:      unorderedGuestProviderName,
		Endpoint:      "https://unordered.example.invalid",
		CredentialRef: "env:unordered",
	}
	path, err := config.ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	if err := config.WriteTo(cfg, path); err != nil {
		t.Fatalf("seed config: %v", err)
	}
}

// the three orders the endpoint produced across the recorded evaluation calls.
var unorderedGuestRotations = [][]string{
	{"proxmox/103", "proxmox/104", "proxmox/102", "proxmox/100", "proxmox/101", "proxmox/105"},
	{"proxmox/100", "proxmox/101", "proxmox/102", "proxmox/103", "proxmox/104", "proxmox/105"},
	{"proxmox/105", "proxmox/104", "proxmox/103", "proxmox/102", "proxmox/101", "proxmox/100"},
}

// listIDs runs a listing command and returns the "id" of each row, in the order
// the command emitted them.
func listIDs(t *testing.T, args ...string) []string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), args, &stdout, &stderr); err != nil {
		t.Fatalf("Run(%v): %v (stderr: %s)", args, err, stderr.String())
	}
	var rows []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &rows); err != nil {
		t.Fatalf("decode %v output %q: %v", args, stdout.String(), err)
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	return ids
}

func listNames(t *testing.T, args ...string) []string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), args, &stdout, &stderr); err != nil {
		t.Fatalf("Run(%v): %v (stderr: %s)", args, err, stderr.String())
	}
	var rows []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &rows); err != nil {
		t.Fatalf("decode %v output %q: %v", args, stdout.String(), err)
	}
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, row.Name)
	}
	return names
}

// TestListLimitIsAPrefixThroughTheCommand exercises the real command path. The
// recorded evaluation showed --limit 3 returning [102 100 101] while --limit 4
// returned [103 102 100 101], so raising the limit dropped 104 and re-added 103.
// The provider here rotates its order on every call, so an unsorted handler
// reproduces that even though the code under test never varies.
func TestListLimitIsAPrefixThroughTheCommand(t *testing.T) {
	for _, kind := range []struct {
		resource string
		idField  string
	}{
		{"container", "id"},
		{"vm", "id"},
	} {
		t.Run(kind.resource, func(t *testing.T) {
			isolateConfigAndHome(t)
			setupUnorderedGuestConfig(t)

			// Each limit below is a separate Run, so each gets a different
			// rotation from the provider. That is the defect: the answer must
			// not depend on which order the endpoint happened to return.
			const total = 6
			var full []string
			for limit := 1; limit <= total+2; limit++ {
				args := []string{"--limit", itoa(limit), "--output", "json", kind.resource, "list"}
				got := listIDs(t, args...)

				if limit == 1 {
					full = listIDs(t, "--output", "json", kind.resource, "list")
					wantFull := []string{"proxmox/100", "proxmox/101", "proxmox/102", "proxmox/103", "proxmox/104", "proxmox/105"}
					if len(full) != len(wantFull) {
						t.Fatalf("unlimited returned %d rows, want %d: %v", len(full), len(wantFull), full)
					}
					for i := range wantFull {
						if full[i] != wantFull[i] {
							t.Fatalf("unlimited row %d = %s, want %s: %v", i, full[i], wantFull[i], full)
						}
					}
				}

				want := total
				if limit < total {
					want = limit
				}
				if len(got) != want {
					t.Fatalf("--limit %d returned %d rows, want %d: %v", limit, len(got), want, got)
				}
				for i := range got {
					if got[i] != full[i] {
						t.Fatalf("--limit %d row %d = %s, want %s (unlimited: %v): %v", limit, i, got[i], full[i], full, got)
					}
				}
			}
		})
	}
}

// TestListLimitIsAPrefixForNodesAndStorages covers the other two listings that
// share the unordered endpoint.
func TestListLimitIsAPrefixForNodesAndStorages(t *testing.T) {
	isolateConfigAndHome(t)
	setupUnorderedGuestConfig(t)

	nodes := listNames(t, "--output", "json", "node", "list")
	if len(nodes) != 3 || nodes[0] != "pve-a" || nodes[2] != "pve-c" {
		t.Fatalf("node list order = %v, want pve-a, pve-b, pve-c", nodes)
	}
	first := listNames(t, "--limit", "2", "--output", "json", "node", "list")
	if len(first) != 2 || first[0] != "pve-a" || first[1] != "pve-b" {
		t.Fatalf("--limit 2 node list = %v, want the first two of %v", first, nodes)
	}

	storages := listNames(t, "--output", "json", "storage", "list")
	if len(storages) != 2 || storages[0] != "backup" || storages[1] != "local" {
		t.Fatalf("storage list order = %v, want backup, local", storages)
	}
	firstStorage := listNames(t, "--limit", "1", "--output", "json", "storage", "list")
	if len(firstStorage) != 1 || firstStorage[0] != "backup" {
		t.Fatalf("--limit 1 storage list = %v, want [backup]", firstStorage)
	}
}

// TestAgentLimitReportsTruncation pins the recorded contradiction through the
// real agent path: --agent --limit 2 over 6 guests emitted "truncated": false
// beside an admission that pagination metadata was unavailable, so a caller
// reading the boolean concluded the 2 returned rows were all 6.
func TestAgentLimitReportsTruncation(t *testing.T) {
	tests := []struct {
		name  string
		limit string
		want  bool
	}{
		{name: "limit met exactly means rows were withheld", limit: "2", want: true},
		{name: "limit met exactly at full count", limit: "6", want: true},
		{name: "limit beyond the row count withheld nothing", limit: "10", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateConfigAndHome(t)
			setupUnorderedGuestConfig(t)

			args := []string{"--agent", "--profile", "unordered"}
			if tt.limit != "" {
				args = append(args, "--limit", tt.limit)
			}
			args = append(args, "container", "list")

			var stdout, stderr bytes.Buffer
			if err := Run(context.Background(), args, &stdout, &stderr); err != nil {
				t.Fatalf("Run(%v): %v (stderr: %s)", args, err, stderr.String())
			}

			var envelope struct {
				Observation struct {
					Completeness string `json:"completeness"`
					Truncated    bool   `json:"truncated"`
					Unsupported  []string
				} `json:"observation"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
				t.Fatalf("decode envelope %q: %v", stdout.String(), err)
			}
			if envelope.Observation.Truncated != tt.want {
				t.Fatalf("--limit %s truncated = %v, want %v (envelope: %s)", tt.limit, envelope.Observation.Truncated, tt.want, stdout.String())
			}
			// The envelope must not claim completeness it cannot prove.
			if envelope.Observation.Completeness != "unknown" {
				t.Fatalf("completeness = %q, want unknown", envelope.Observation.Completeness)
			}
		})
	}
}

// itoa avoids importing strconv purely for the test table.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}
