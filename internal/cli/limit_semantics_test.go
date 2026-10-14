package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/geoffmcc/nodex/internal/domain"
)

// --- --limit semantics (finding F06) ---
//
// `--limit 10 node services` returned all 23 services: the handler ignored the
// limit, while other list handlers (for example `container list`) bounded the
// output and marked the read truncated. The flag must bound every provider
// inventory list, and agent mode must report truncation truthfully.

func fiveNodeServices() []domain.NodeService {
	return []domain.NodeService{
		{Name: "chrony", State: "running", Active: true},
		{Name: "corosync", State: "dead", Active: false},
		{Name: "cron", State: "running", Active: true},
		{Name: "ssh", State: "running", Active: true},
		{Name: "pveproxy", State: "running", Active: true},
	}
}

// TestNodeServicesHonorsLimitPlainMode pins the concrete defect: the plain
// (non-agent) list must be bounded by --limit, exactly as container list is.
func TestNodeServicesHonorsLimitPlainMode(t *testing.T) {
	isolateConfigAndHome(t)
	setupE2EConfig(t)

	e2eNodeServicesOverride = fiveNodeServices()
	t.Cleanup(func() { e2eNodeServicesOverride = nil })

	var out, errOut bytes.Buffer
	if err := Run(context.Background(), []string{"--output", "json", "--limit", "2", "node", "services", "e2e-node"}, &out, &errOut); err != nil {
		t.Fatalf("node services: %v (stderr: %s)", err, errOut.String())
	}
	var services []domain.NodeService
	if err := json.Unmarshal(out.Bytes(), &services); err != nil {
		t.Fatalf("decode services %q: %v", out.String(), err)
	}
	if len(services) != 2 {
		t.Fatalf("--limit 2 returned %d services, want 2", len(services))
	}
}

// TestNodeServicesLimitZeroReturnsAll: no limit keeps the full list, so the
// default behavior is unchanged.
func TestNodeServicesLimitZeroReturnsAll(t *testing.T) {
	isolateConfigAndHome(t)
	setupE2EConfig(t)

	e2eNodeServicesOverride = fiveNodeServices()
	t.Cleanup(func() { e2eNodeServicesOverride = nil })

	var out, errOut bytes.Buffer
	if err := Run(context.Background(), []string{"--output", "json", "node", "services", "e2e-node"}, &out, &errOut); err != nil {
		t.Fatalf("node services: %v (stderr: %s)", err, errOut.String())
	}
	var services []domain.NodeService
	if err := json.Unmarshal(out.Bytes(), &services); err != nil {
		t.Fatalf("decode services %q: %v", out.String(), err)
	}
	if len(services) != 5 {
		t.Fatalf("no limit returned %d services, want all 5", len(services))
	}
}

// TestAgentNodeServicesReportsTruncation closes the recorded contradiction: a
// limited list whose handler dropped rows must report truncated:true, and a
// limit the data does not reach must report false.
func TestAgentNodeServicesReportsTruncation(t *testing.T) {
	tests := []struct {
		name  string
		limit string
		want  bool
	}{
		{name: "limit met exactly means rows may have been withheld", limit: "2", want: true},
		{name: "limit beyond the row count withheld nothing", limit: "10", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateConfigAndHome(t)
			setupE2EConfig(t)

			e2eNodeServicesOverride = fiveNodeServices()
			t.Cleanup(func() { e2eNodeServicesOverride = nil })

			var stdout, stderr bytes.Buffer
			args := []string{"--agent", "--profile", "e2e", "--limit", tt.limit, "node", "services", "e2e-node"}
			if err := Run(context.Background(), args, &stdout, &stderr); err != nil {
				t.Fatalf("Run(%v): %v (stderr: %s)", args, err, stderr.String())
			}

			var envelope struct {
				Data        []domain.NodeService `json:"data"`
				Observation struct {
					Completeness string `json:"completeness"`
					Truncated    bool   `json:"truncated"`
				} `json:"observation"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
				t.Fatalf("decode envelope %q: %v", stdout.String(), err)
			}
			if envelope.Observation.Truncated != tt.want {
				t.Fatalf("--limit %s truncated = %v, want %v (envelope: %s)", tt.limit, envelope.Observation.Truncated, tt.want, stdout.String())
			}
			if tt.want {
				// When the limit is reached the returned rows must actually be bounded.
				if len(envelope.Data) != 2 {
					t.Fatalf("--limit 2 returned %d rows, want 2", len(envelope.Data))
				}
			}
			if envelope.Observation.Completeness != "unknown" {
				t.Fatalf("completeness = %q, want unknown", envelope.Observation.Completeness)
			}
		})
	}
}
