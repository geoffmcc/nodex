package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/output"
)

func capacityFixture(name, key string) domain.Storage {
	s := domain.Storage{ID: "storage/node/" + name, Name: name, Node: "node", Type: "pbs", Status: "available", Total: 100, Used: 25, Avail: 75}
	if key != "" {
		s.CapacityIdentity = &domain.StorageCapacityIdentity{Key: key, Scope: "pbs_datastore", Evidence: "pve_storage_config_server_port_datastore"}
	}
	return s
}

func TestStatusStorageGrouping(t *testing.T) {
	rows := []domain.Storage{capacityFixture("pbs-shutdown", "same"), capacityFixture("pbs", "same"), capacityFixture("pbs-nightly", "same"), capacityFixture("different-store", "different"), capacityFixture("local", ""), capacityFixture("local-lvm", "")}
	rows[2].CapacityIdentity.Namespace = "nightly"
	rows[4].Type = "dir"
	rows[4].Backing = &domain.StorageBacking{Type: "dir", Path: "/var/lib/vz"}
	rows[5].Type = "lvmthin"
	rows[5].Backing = &domain.StorageBacking{Type: "lvmthin", VolumeGroup: "pve", ThinPool: "data"}
	when := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	entries, capacity := buildStatusStorage(rows, "available", when)
	if rows[0].Name != "pbs-shutdown" {
		t.Fatal("mutated provider slice")
	}
	if len(entries) != 6 || len(capacity.Groups) != 2 || len(capacity.UnresolvedEntries) != 2 {
		t.Fatalf("incorrect grouping: %+v", capacity)
	}
	var shared statusCapacityGroup
	for _, g := range capacity.Groups {
		if g.ID == "same" {
			shared = g
		}
	}
	if len(shared.Members) != 3 || shared.Metrics == nil || shared.Metrics.Total != 100 || shared.Metrics.Used != 25 {
		t.Fatalf("summed/lost alias metrics: %+v", shared)
	}
	if capacity.PhysicalTotalState != "not_established" || capacity.SourceFreshness != "unknown" || capacity.CollectedAt != when.Format(time.RFC3339Nano) {
		t.Fatalf("invented backing/freshness: %+v", capacity)
	}
	var human bytes.Buffer
	overview := &statusOverview{Storage: entries, StorageCapacity: capacity}
	overview.DiskInventory = &statusDiskInventory{State: "available", Nodes: 1, NodesReported: 1, DeviceCount: 1, TotalBytes: 68719476736, TotalHuman: "64.0 GiB", Devices: []statusNodeDisk{{Node: "node", Name: "/dev/sda", Size: 68719476736}}}
	if err := writeStatusStorage(&human, overview); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Configured backend: directory /var/lib/vz\n\n", "Configured backend: LVM thinpool pve/data\n\n", "[available]\n\n  Disk inventory:"} {
		if !strings.Contains(human.String(), want) {
			t.Fatalf("storage blocks missing separation %q:\n%s", want, human.String())
		}
	}
	for _, want := range []string{"pbs-shutdown", "pbs-nightly", "node/pbs ", "namespace: nightly", "verified configured aliases/views", "Configured backend: directory /var/lib/vz", "Configured backend: LVM thinpool pve/data", "Disk inventory: 1 device, 64.0 GiB (pools unmapped)", "Storage capacities may overlap; do not add them"} {
		if !strings.Contains(human.String(), want) {
			t.Fatalf("missing %q:\n%s", want, human.String())
		}
	}
	for _, jargon := range []string{"relationship unresolved", "identity_not_established", "backing topology unverified", "Per-entry/group capacities", "Physical disk totals: unavailable"} {
		if strings.Contains(human.String(), jargon) {
			t.Fatalf("human output contains internal wording %q:\n%s", jargon, human.String())
		}
	}
	// The three aliases display capacity once; the other three entries once each.
	if strings.Count(human.String(), "(25.0%)") != 4 {
		t.Fatal(human.String())
	}
	for _, format := range []output.Format{output.FormatJSON, output.FormatYAML} {
		var buf bytes.Buffer
		if err := writeStatus(&Context{Writer: &buf, Opts: Options{Output: format}}, overview); err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"capacity_group", "pbs-nightly", "nightly", "source_freshness"} {
			if !strings.Contains(buf.String(), want) {
				t.Fatalf("%s missing %s", format, want)
			}
		}
	}
	raw, _ := json.Marshal(capacity)
	if err := validateAgainstJSONSchema(filepath.Join("..", "..", "docs", "schemas", "status-storage-capacity-v1.schema.json"), raw); err != nil {
		t.Fatal(err)
	}
	// Same snapshot yields stable grouping and ordering regardless of API order.
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
	entries2, capacity2 := buildStatusStorage(rows, "available", when)
	if !reflect.DeepEqual(entries, entries2) || !reflect.DeepEqual(capacity, capacity2) {
		t.Fatal("unstable snapshot order")
	}
}

func TestStatusStorageUncertainMetrics(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*domain.Storage)
		state  string
	}{
		{"different usage", func(s *domain.Storage) { s.Used++ }, "conflicting_views"},
		{"different total", func(s *domain.Storage) { s.Total++ }, "conflicting_views"},
		{"disconnected", func(s *domain.Storage) { s.Status = "unknown" }, "unavailable_views"},
		{"negative usage", func(s *domain.Storage) { s.Used = -1 }, "unavailable_views"},
		{"thin overprovisioned", func(s *domain.Storage) { s.Used = 200 }, "unavailable_views"},
		{"zero capacity", func(s *domain.Storage) { s.Total = 0 }, "unavailable_views"},
		{"disabled", func(s *domain.Storage) { disabled := false; s.CapacityIdentity.Enabled = &disabled }, "unavailable_views"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows := []domain.Storage{capacityFixture("a", "same"), capacityFixture("b", "same")}
			tc.change(&rows[1])
			entries, c := buildStatusStorage(rows, "available", time.Now())
			if len(c.Groups) != 1 || c.Groups[0].Metrics != nil || c.Groups[0].MetricsState != tc.state {
				t.Fatalf("unreliable group metric: %+v", c.Groups)
			}
			if entries[1].Used != rows[1].Used {
				t.Fatal("per-entry value changed")
			}
			var buf bytes.Buffer
			if err := writeStatusStorage(&buf, &statusOverview{Storage: entries, StorageCapacity: c}); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(buf.String(), "per-entry metrics only") {
				t.Fatal(buf.String())
			}
		})
	}
}

func TestStatusPBSMinorDifferencesDoNotDegradeHealth(t *testing.T) {
	const total = int64(1024 * 1024 * 1024)
	for _, tc := range []struct {
		name        string
		change      func(*domain.Storage)
		wantHealthy bool
	}{
		{"observed 4 KiB", func(s *domain.Storage) { s.Used += 4096; s.Avail -= 4096 }, true},
		{"threshold", func(s *domain.Storage) { s.Used += 65536; s.Avail -= 65536 }, true},
		{"above threshold", func(s *domain.Storage) { s.Used += 65537; s.Avail -= 65537 }, false},
		{"different total", func(s *domain.Storage) { s.Total++ }, false},
		{"unavailable view", func(s *domain.Storage) { s.Status = "unknown" }, false},
		{"other storage scope", func(s *domain.Storage) { s.Used++; s.CapacityIdentity.Scope = "shared_storage" }, false},
		{"mixed storage scopes", func(s *domain.Storage) { s.Used++; s.CapacityIdentity.Scope = "shared_storage" }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows := []domain.Storage{capacityFixture("a", "same"), capacityFixture("b", "same")}
			for i := range rows {
				rows[i].Total = total
				rows[i].Used = total / 4
				rows[i].Avail = total * 3 / 4
			}
			tc.change(&rows[1])
			if tc.name == "other storage scope" {
				rows[0].CapacityIdentity.Scope = "shared_storage"
			}
			entries, capacity := buildStatusStorage(rows, "available", time.Now())
			snapshot := healthyWatchSnapshot()
			snapshot.Storage, snapshot.StorageCapacity = entries, capacity
			health, _ := statusHealthDetail(snapshot)
			if (health == "healthy") != tc.wantHealthy {
				t.Fatalf("health = %q, capacity = %+v", health, capacity)
			}
			if capacity.Groups[0].Metrics != nil {
				t.Fatal("conflicting values must never be presented as a single metric")
			}
			if tc.wantHealthy {
				if capacity.Groups[0].MetricsState != "conflicting_views" {
					t.Fatal("structured conflict was lost")
				}
				var human bytes.Buffer
				if err := writeStatusStorage(&human, snapshot); err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(human.String(), "capacity views differ slightly (within 64 KiB); per-entry metrics only") {
					t.Fatal(human.String())
				}
				frame := output.SanitizeTerminal(renderStatusWatch("lab", statusWatchView{Snapshot: snapshot, Health: health}, 120, 40))
				if !strings.Contains(frame, "PBS views differ slightly") {
					t.Fatalf("missing watch indicator:\n%s", frame)
				}
			}
		})
	}
	rows := []domain.Storage{capacityFixture("a", "same"), capacityFixture("b", "same")}
	for i := range rows {
		rows[i].Total = 1024 * 1024
		rows[i].Used = 100000
		rows[i].Avail = 900000
	}
	rows[1].Used += 1024
	entries, capacity := buildStatusStorage(rows, "available", time.Now())
	if statusPBSMinorDifference(capacity.Groups[0], entries) {
		t.Fatal("large relative difference on a small datastore was accepted")
	}
	entries[1].Used = entries[0].Used + 50
	if !statusPBSMinorDifference(capacity.Groups[0], entries) {
		t.Fatal("small difference on a small datastore was rejected")
	}
	capacity.Groups[0].Members = append(capacity.Groups[0].Members, len(entries))
	if statusPBSMinorDifference(capacity.Groups[0], entries) {
		t.Fatal("invalid member index was accepted")
	}
}

type failingStorageProvider struct{ bareProvider }

func (*failingStorageProvider) Storage(context.Context) ([]domain.Storage, error) {
	return nil, errors.New("password=secret")
}
func (*failingStorageProvider) StorageContent(context.Context, string, string) ([]domain.StorageContentItem, error) {
	panic("unexpected content query")
}

func TestStatusStorageFailureAndSanitization(t *testing.T) {
	for _, tc := range []struct {
		prov domain.Provider
		want string
	}{{&failingStorageProvider{}, "unavailable"}, {&bareProvider{}, "unsupported"}} {
		var overview statusOverview
		collectStatusStorage(t.Context(), tc.prov, &overview)
		if overview.StorageCapacity.State != tc.want {
			t.Fatal(overview.StorageCapacity)
		}
		var buf bytes.Buffer
		if err := writeStatusStorage(&buf, &overview); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(buf.String(), tc.want) || strings.Contains(buf.String(), "secret") {
			t.Fatal(buf.String())
		}
	}
	s := capacityFixture("evil\x1b[2J\npassword=secret", "")
	entries, c := buildStatusStorage([]domain.Storage{s}, "available", time.Now())
	for _, format := range []output.Format{output.FormatTable, output.FormatJSON, output.FormatYAML} {
		var buf bytes.Buffer
		if err := writeStatus(&Context{Writer: &buf, Opts: Options{Output: format}}, &statusOverview{Storage: entries, StorageCapacity: c}); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(buf.String(), "\x1b") || strings.Contains(buf.String(), "secret") {
			t.Fatal("unsafe output", buf.String())
		}
	}
	if err := writeStatusStorage(errorWriter{}, &statusOverview{Storage: entries, StorageCapacity: c}); err == nil {
		t.Fatal("write failure swallowed")
	}
}

type errorWriter struct{}

func (errorWriter) Write([]byte) (int, error) { return 0, errors.New("broken pipe") }
