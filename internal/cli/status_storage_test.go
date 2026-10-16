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
	rows[5].Type = "lvmthin"
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
	if err := writeStatusStorage(&human, overview); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"pbs-shutdown", "pbs-nightly", "node/pbs ", "namespace: nightly", "verified configured aliases/views", "Physical disk totals: not established", "relationship unresolved"} {
		if !strings.Contains(human.String(), want) {
			t.Fatalf("missing %q:\n%s", want, human.String())
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
