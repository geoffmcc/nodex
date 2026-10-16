package cli

import (
	"context"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/output"
)

// This additive versioned object leaves storage[] numeric fields as per-entry
// values. A capacity group is not an independent physical disk or a summand.
type statusStorageCapacity struct {
	Schema              int                   `json:"schema" yaml:"schema"`
	State               string                `json:"state" yaml:"state"`
	CollectedAt         string                `json:"collected_at" yaml:"collected_at"`
	SourceFreshness     string                `json:"source_freshness" yaml:"source_freshness"`
	Groups              []statusCapacityGroup `json:"groups" yaml:"groups"`
	UnresolvedEntries   []int                 `json:"unresolved_entries" yaml:"unresolved_entries"`
	PhysicalTotalState  string                `json:"physical_total_state" yaml:"physical_total_state"`
	PhysicalTotalReason string                `json:"physical_total_reason" yaml:"physical_total_reason"`
}

type statusCapacityGroup struct {
	ID           string                 `json:"id" yaml:"id"`
	Scope        string                 `json:"scope" yaml:"scope"`
	Evidence     string                 `json:"evidence" yaml:"evidence"`
	Members      []int                  `json:"members" yaml:"members"`
	MetricsState string                 `json:"metrics_state" yaml:"metrics_state"`
	Metrics      *statusCapacityMetrics `json:"metrics,omitempty" yaml:"metrics,omitempty"`
}

type statusCapacityMetrics struct {
	Total int64 `json:"total" yaml:"total"`
	Used  int64 `json:"used" yaml:"used"`
	Avail int64 `json:"avail" yaml:"avail"`
}

func collectStatusStorage(ctx context.Context, prov domain.Provider, overview *statusOverview) {
	state := "unsupported"
	var storages []domain.Storage
	if si, ok := prov.(domain.StorageInspector); ok {
		var err error
		storages, err = si.Storage(ctx)
		state = "available"
		if err != nil {
			state = "unavailable"
			storages = nil
		}
	}
	overview.Storage, overview.StorageCapacity = buildStatusStorage(storages, state, time.Now())
}

func buildStatusStorage(storages []domain.Storage, state string, collected time.Time) ([]statusStorage, *statusStorageCapacity) {
	capacity := &statusStorageCapacity{
		Schema: 1, State: state, CollectedAt: collected.UTC().Format(time.RFC3339Nano),
		SourceFreshness: "unknown", Groups: []statusCapacityGroup{}, UnresolvedEntries: []int{},
		PhysicalTotalState: "not_established", PhysicalTotalReason: "backing_topology_unverified",
	}
	// Copy before sorting; provider-owned values can be reused by other views.
	storages = append([]domain.Storage(nil), storages...)
	sort.SliceStable(storages, func(i, j int) bool {
		a, b := storages[i], storages[j]
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		if a.Node != b.Node {
			return a.Node < b.Node
		}
		return a.ID < b.ID
	})
	var entries []statusStorage
	if len(storages) > 0 {
		entries = make([]statusStorage, 0, len(storages))
	}
	groups := make(map[string]int)
	for _, s := range storages {
		identity := domain.StorageCapacityIdentity{Scope: "unknown", Evidence: "none", Reason: "identity_not_established"}
		if s.CapacityIdentity != nil {
			identity = *s.CapacityIdentity
		}
		entry := statusStorage{
			Name: s.Name, Type: s.Type, ID: s.ID, Node: s.Node, Status: s.Status,
			Total: s.Total, Used: s.Used, Avail: s.Avail,
			TotalHuman: formatBytes(s.Total), UsedHuman: formatBytes(s.Used), AvailHuman: formatBytes(s.Avail),
			Identity: &identity, Backing: s.Backing,
		}
		index := len(entries)
		if identity.Key == "" || identity.Evidence == "none" || identity.Scope == "unknown" {
			capacity.UnresolvedEntries = append(capacity.UnresolvedEntries, index)
		} else {
			entry.CapacityGroup = identity.Key
			gi, exists := groups[identity.Key]
			if !exists {
				gi = len(capacity.Groups)
				groups[identity.Key] = gi
				capacity.Groups = append(capacity.Groups, statusCapacityGroup{
					ID: identity.Key, Scope: identity.Scope, Evidence: identity.Evidence,
					MetricsState: "consistent_views", Metrics: &statusCapacityMetrics{Total: s.Total, Used: s.Used, Avail: s.Avail},
				})
			}
			group := &capacity.Groups[gi]
			group.Members = append(group.Members, index)
			if !statusStorageMetricsAvailable(entry) {
				group.MetricsState, group.Metrics = "unavailable_views", nil
			} else if m := group.Metrics; m != nil && (m.Total != s.Total || m.Used != s.Used || m.Avail != s.Avail) {
				// Equality is only a consistency check AFTER identity matching.
				group.MetricsState, group.Metrics = "conflicting_views", nil
			}
		}
		entries = append(entries, entry)
	}
	return entries, capacity
}

func statusStorageMetricsAvailable(s statusStorage) bool {
	return (s.Status == "available" || s.Status == "active") &&
		(s.Identity == nil || s.Identity.Enabled == nil || *s.Identity.Enabled) &&
		s.Total > 0 && s.Used >= 0 && s.Used <= s.Total && s.Avail >= 0 && s.Avail <= s.Total
}

func statusStorageUsage(s statusStorage) string {
	if !statusStorageMetricsAvailable(s) {
		return "capacity unavailable (per-entry values retained in structured output)"
	}
	pct := float64(s.Used) / float64(s.Total) * 100
	warning := ""
	if pct > 90 {
		warning = " [WARN]"
	} else if pct > 75 {
		warning = " [HIGH]"
	}
	return fmt.Sprintf("%s / %s (%.1f%%)%s", formatBytes(s.Used), formatBytes(s.Total), pct, warning)
}

func writeStatusStorage(w io.Writer, overview *statusOverview) error {
	// Also protect direct renderer callers, not only the top-level CLI writer.
	w = output.NewSanitizingWriter(w)
	capacity := overview.StorageCapacity
	if capacity == nil && len(overview.Storage) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w, "\nStorage:"); err != nil {
		return err
	}
	if capacity != nil && capacity.State != "available" {
		_, err := fmt.Fprintf(w, "  %s (no capacity snapshot)\n", capacity.State)
		return err
	}
	groupByID := make(map[string]statusCapacityGroup)
	if capacity != nil {
		for _, group := range capacity.Groups {
			groupByID[group.ID] = group
		}
	}
	printed := make(map[string]bool)
	for _, entry := range overview.Storage {
		group, grouped := groupByID[entry.CapacityGroup]
		if grouped && len(group.Members) > 1 {
			if printed[group.ID] {
				continue
			}
			printed[group.ID] = true
			label := "Shared storage (verified node views)"
			if group.Scope == "pbs_datastore" {
				label = "PBS datastore (verified configured aliases/views)"
			}
			if _, err := fmt.Fprintf(w, "  %s\n", label); err != nil {
				return err
			}
			if group.Metrics != nil {
				if _, err := fmt.Fprintf(w, "    %s\n", statusStorageUsage(entry)); err != nil {
					return err
				}
			} else {
				if _, err := fmt.Fprintf(w, "    %s; per-entry metrics only\n", group.MetricsState); err != nil {
					return err
				}
			}
			for _, index := range group.Members {
				s := overview.Storage[index]
				if err := writeStatusStorageEntry(w, s, "    ", group.Metrics == nil); err != nil {
					return err
				}
			}
		} else if err := writeStatusStorageEntry(w, entry, "  ", true); err != nil {
			return err
		}
	}
	if err := writeStatusDiskInventory(w, overview.DiskInventory); err != nil {
		return err
	}
	_, err := fmt.Fprintln(w, "  Storage capacities may overlap; do not add them.")
	return err
}

func writeStatusStorageEntry(w io.Writer, s statusStorage, indent string, metrics bool) error {
	name := s.Name
	if s.Node != "" {
		name = s.Node + "/" + name
	}
	if _, err := fmt.Fprintf(w, "%s%s  %s", indent, name, s.Type); err != nil {
		return err
	}
	if s.Identity != nil && (s.Identity.Scope == "pbs_datastore" || s.Identity.Namespace != "") {
		ns := s.Identity.Namespace
		if ns == "" {
			ns = "(root)"
		}
		if _, err := fmt.Fprintf(w, "  namespace: %s", ns); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "  [%s]\n", s.Status); err != nil {
		return err
	}
	if metrics {
		if _, err := fmt.Fprintf(w, "%s  %s\n", indent, statusStorageUsage(s)); err != nil {
			return err
		}
	}
	if label := storageBackingLabel(s.Backing); label != "" {
		if _, err := fmt.Fprintf(w, "%s  Configured backend: %s\n", indent, label); err != nil {
			return err
		}
	}
	return nil
}

func storageBackingLabel(b *domain.StorageBacking) string {
	if b == nil {
		return ""
	}
	switch b.Type {
	case "dir":
		if b.Path != "" {
			return "directory " + b.Path
		}
	case "lvmthin":
		if b.VolumeGroup != "" && b.ThinPool != "" {
			return "LVM thinpool " + b.VolumeGroup + "/" + b.ThinPool
		}
		if b.VolumeGroup != "" {
			return "LVM volume group " + b.VolumeGroup
		}
	case "zfspool":
		if b.ZFSPool != "" {
			return "ZFS pool " + b.ZFSPool
		}
	}
	return ""
}

func collectStatusDiskInventory(ctx context.Context, prov domain.Provider, overview *statusOverview) {
	inventory := &statusDiskInventory{State: "unsupported", Devices: []statusNodeDisk{}}
	detail, ok := prov.(domain.NodeDetailProvider)
	if !ok {
		overview.DiskInventory = inventory
		return
	}
	if overview.Sections["nodes"] != "available" {
		inventory.State = "unavailable"
		overview.DiskInventory = inventory
		return
	}
	nodes := append([]statusNode(nil), overview.NodesDetail...)
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Name < nodes[j].Name })
	if len(nodes) == 0 {
		inventory.State = "unavailable"
		overview.DiskInventory = inventory
		return
	}
	inventory.Nodes = len(nodes)
	for _, node := range nodes {
		if node.Status != "online" || ctx.Err() != nil {
			continue
		}
		disks, err := detail.NodeDisks(ctx, node.Name)
		if err != nil {
			continue
		}
		inventory.NodesReported++
		for _, disk := range disks {
			name := disk.Name
			if name == "" {
				name = disk.Path
			}
			inventory.Devices = append(inventory.Devices, statusNodeDisk{
				Node: node.Name, Name: name, Path: disk.Path, Size: disk.Size,
				Type: disk.Type, Model: disk.Model, Health: disk.Health,
			})
			if disk.Size > 0 {
				inventory.TotalBytes += disk.Size
			}
		}
	}
	inventory.DeviceCount = len(inventory.Devices)
	if inventory.DeviceCount > 0 {
		inventory.TotalHuman = formatBytes(inventory.TotalBytes)
	}
	switch {
	case inventory.NodesReported == 0:
		inventory.State = "unavailable"
	case inventory.NodesReported < inventory.Nodes:
		inventory.State = "partial"
	default:
		inventory.State = "available"
	}
	overview.DiskInventory = inventory
}

func writeStatusDiskInventory(w io.Writer, inventory *statusDiskInventory) error {
	if inventory == nil {
		_, err := fmt.Fprintln(w, "  Disk inventory: unavailable")
		return err
	}
	if inventory.State != "available" && inventory.State != "partial" {
		_, err := fmt.Fprintf(w, "  Disk inventory: %s\n", inventory.State)
		return err
	}
	if inventory.DeviceCount == 0 {
		_, err := fmt.Fprintf(w, "  Disk inventory: no devices reported (%s)\n", inventory.State)
		return err
	}
	deviceLabel := "device"
	if inventory.DeviceCount != 1 {
		deviceLabel = "devices"
	}
	partial := ""
	if inventory.State == "partial" {
		partial = "partial; "
	}
	_, err := fmt.Fprintf(w, "  Disk inventory: %d %s, %s (%spools unmapped)\n", inventory.DeviceCount, deviceLabel, inventory.TotalHuman, partial)
	return err
}
