package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/output"
)

// clusterStatusStub is a minimal provider that only implements the optional
// ClusterStatusProvider capability, so detectQuorum can be exercised without
// standing up a full mock.
type clusterStatusStub struct {
	bareProvider
	items []domain.ClusterStatusDetail
	err   error
}

func (c *clusterStatusStub) ClusterStatuses(context.Context) ([]domain.ClusterStatusDetail, error) {
	return c.items, c.err
}

func TestDetectQuorum(t *testing.T) {
	tests := []struct {
		name        string
		prov        any
		wantStand   bool
		wantKnown   bool
		wantQuorum  int
		wantName    string
		wantDisplay string
	}{
		{
			name:        "cluster with quorum",
			prov:        &clusterStatusStub{items: []domain.ClusterStatusDetail{{Type: "cluster", Name: "prod", Quorate: 3}}},
			wantKnown:   true,
			wantQuorum:  3,
			wantName:    "prod",
			wantDisplay: "3",
		},
		{
			name:        "cluster formed but no quorum is still known",
			prov:        &clusterStatusStub{items: []domain.ClusterStatusDetail{{Type: "cluster", Name: "prod", Quorate: 0}}},
			wantKnown:   true,
			wantQuorum:  0,
			wantName:    "prod",
			wantDisplay: "0",
		},
		{
			name:        "standalone: nodes but no cluster entry",
			prov:        &clusterStatusStub{items: []domain.ClusterStatusDetail{{Type: "node", Name: "pve1", Status: "online"}}},
			wantStand:   true,
			wantDisplay: "n/a (standalone host)",
		},
		{
			name:        "empty response means standalone",
			prov:        &clusterStatusStub{items: nil},
			wantStand:   true,
			wantDisplay: "n/a (standalone host)",
		},
		{
			// A failed query must not be reported as "not clustered".
			name:        "query failure is unknown, not standalone",
			prov:        &clusterStatusStub{err: errors.New("permission denied")},
			wantDisplay: "unavailable",
		},
		{
			name:        "provider without the capability is unknown",
			prov:        &bareProvider{},
			wantDisplay: "unavailable",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectQuorum(context.Background(), tt.prov)
			if got.Standalone != tt.wantStand {
				t.Errorf("Standalone = %v, want %v", got.Standalone, tt.wantStand)
			}
			if got.Known != tt.wantKnown {
				t.Errorf("Known = %v, want %v", got.Known, tt.wantKnown)
			}
			if got.Quorum != tt.wantQuorum {
				t.Errorf("Quorum = %d, want %d", got.Quorum, tt.wantQuorum)
			}
			if got.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", got.Name, tt.wantName)
			}
			if display := got.quorumDisplay(); display != tt.wantDisplay {
				t.Errorf("quorumDisplay() = %q, want %q", display, tt.wantDisplay)
			}
		})
	}
}

func TestAnnotateStandaloneServices(t *testing.T) {
	services := []domain.NodeService{
		{Name: "corosync", State: "dead"},
		{Name: "pmxcfs", State: "inactive"},
		{Name: "pveproxy", State: "running", Active: true},
		{Name: "ssh", State: "running", Active: true},
	}

	t.Run("standalone annotates cluster-only services", func(t *testing.T) {
		svcs := append([]domain.NodeService(nil), services...)
		annotateStandaloneServices(svcs, quorumInfo{Standalone: true})
		if !svcs[0].NotApplicable || svcs[0].Note != standaloneNote {
			t.Errorf("corosync = %+v, want NotApplicable with note %q", svcs[0], standaloneNote)
		}
		if !svcs[1].NotApplicable || svcs[1].Note != standaloneNote {
			t.Errorf("pmxcfs = %+v, want NotApplicable with note %q", svcs[1], standaloneNote)
		}
		// The raw provider values must survive for scripted use.
		if svcs[0].State != "dead" {
			t.Errorf("corosync state = %q, want the reported %q", svcs[0].State, "dead")
		}
		for _, idx := range []int{2, 3} {
			if svcs[idx].NotApplicable || svcs[idx].Note != "" {
				t.Errorf("%s = %+v, want no annotation", svcs[idx].Name, svcs[idx])
			}
		}
	})

	t.Run("clustered host is left alone", func(t *testing.T) {
		svcs := append([]domain.NodeService(nil), services...)
		annotateStandaloneServices(svcs, quorumInfo{Known: true, Quorum: 3})
		for _, s := range svcs {
			if s.NotApplicable || s.Note != "" {
				t.Errorf("%s = %+v, want no annotation on a clustered host", s.Name, s)
			}
		}
	})

	t.Run("unknown cluster state is left alone", func(t *testing.T) {
		svcs := append([]domain.NodeService(nil), services...)
		annotateStandaloneServices(svcs, quorumInfo{})
		for _, s := range svcs {
			if s.NotApplicable {
				t.Errorf("%s = %+v, want no annotation when cluster state is unknown", s.Name, s)
			}
		}
	})
}

func TestWriteNodeServicesNoteColumn(t *testing.T) {
	services := []domain.NodeService{
		{Name: "corosync", State: "dead", NotApplicable: true, Note: standaloneNote},
		{Name: "pveproxy", State: "running", Active: true},
	}
	var buf bytes.Buffer
	cmdCtx := &Context{Writer: &buf, Opts: Options{Output: output.FormatTable}}
	if err := writeNodeServices(cmdCtx, services); err != nil {
		t.Fatalf("writeNodeServices: %v", err)
	}
	got := buf.String()
	for _, want := range []string{"NOTE", "not applicable (standalone host)", "yes", "no"} {
		if !strings.Contains(got, want) {
			t.Errorf("table output missing %q:\n%s", want, got)
		}
	}
}

func TestNewHAStatusView(t *testing.T) {
	tests := []struct {
		name       string
		status     *domain.HAStatus
		quorum     quorumInfo
		wantEnable bool
		wantClust  string
		wantStand  bool
		wantKnown  bool
	}{
		{
			name:       "clustered host has HA enabled",
			status:     &domain.HAStatus{Quorum: 1, Status: "online"},
			quorum:     quorumInfo{Known: true, Name: "prod", Quorum: 1},
			wantEnable: true,
			wantClust:  "prod",
			wantKnown:  true,
		},
		{
			// The nit: a standalone host must not look like a broken cluster.
			name:       "standalone host has HA disabled",
			status:     &domain.HAStatus{Quorum: 0, Status: "unknown"},
			quorum:     quorumInfo{Standalone: true},
			wantEnable: false,
			wantClust:  "",
			wantStand:  true,
		},
		{
			name:   "nil status is tolerated",
			status: nil,
			quorum: quorumInfo{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			view := newHAStatusView(tt.status, tt.quorum)
			if view.Enabled != tt.wantEnable {
				t.Errorf("Enabled = %v, want %v", view.Enabled, tt.wantEnable)
			}
			if view.Cluster != tt.wantClust {
				t.Errorf("Cluster = %q, want %q", view.Cluster, tt.wantClust)
			}
			if view.Standalone != tt.wantStand {
				t.Errorf("Standalone = %v, want %v", view.Standalone, tt.wantStand)
			}
			if view.QuorumKnown != tt.wantKnown {
				t.Errorf("QuorumKnown = %v, want %v", view.QuorumKnown, tt.wantKnown)
			}
		})
	}
}

func TestWriteHAStatusTableStandalone(t *testing.T) {
	var buf bytes.Buffer
	cmdCtx := &Context{Writer: &buf, Opts: Options{Output: output.FormatTable}}
	view := &haStatusView{Status: "unknown", Standalone: true}
	if err := writeHAStatusTable(cmdCtx, view); err != nil {
		t.Fatalf("writeHAStatusTable: %v", err)
	}
	got := buf.String()
	for _, want := range []string{"ENABLED", "CLUSTER", "false", "n/a (standalone host)"} {
		if !strings.Contains(got, want) {
			t.Errorf("table output missing %q:\n%s", want, got)
		}
	}
}
