package cli

import (
	"time"

	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/output"
)

// formatUptime renders a duration in a human-friendly form, e.g. "217h0m0s".
func formatUptime(d time.Duration) string {
	return d.String()
}

// decorateNode fills the human-readable companion fields derived from raw
// machine values. It copies the struct so callers never mutate shared slices.
func decorateNode(n domain.Node) domain.Node {
	if n.Uptime != nil {
		n.UptimeSeconds = int64(*n.Uptime / time.Second)
		n.UptimeHuman = formatUptime(*n.Uptime)
	}
	return n
}

// decorateNodes returns a decorated copy of each node in s.
func decorateNodes(s []domain.Node) []domain.Node {
	out := make([]domain.Node, len(s))
	for i, n := range s {
		out[i] = decorateNode(n)
	}
	return out
}

// decorateVM fills MemoryHuman and DiskHuman derived from raw byte counts.
func decorateVM(v domain.VM) domain.VM {
	v.MemoryHuman = formatBytes(v.Memory)
	v.DiskHuman = formatBytes(v.Disk)
	return v
}

// decorateVMs returns a decorated copy of each VM in s.
func decorateVMs(s []domain.VM) []domain.VM {
	out := make([]domain.VM, len(s))
	for i, v := range s {
		out[i] = decorateVM(v)
	}
	return out
}

// decorateContainer fills MemoryHuman and DiskHuman derived from raw byte counts.
func decorateContainer(c domain.Container) domain.Container {
	c.MemoryHuman = formatBytes(c.Memory)
	c.DiskHuman = formatBytes(c.Disk)
	return c
}

// decorateContainers returns a decorated copy of each container in s.
func decorateContainers(s []domain.Container) []domain.Container {
	out := make([]domain.Container, len(s))
	for i, c := range s {
		out[i] = decorateContainer(c)
	}
	return out
}

// decorateStorage fills the *_human byte fields derived from raw counts.
func decorateStorage(s domain.Storage) domain.Storage {
	s.TotalHuman = formatBytes(s.Total)
	s.UsedHuman = formatBytes(s.Used)
	s.AvailHuman = formatBytes(s.Avail)
	return s
}

// decorateStorages returns a decorated copy of each storage in s.
func decorateStorages(s []domain.Storage) []domain.Storage {
	out := make([]domain.Storage, len(s))
	for i, st := range s {
		out[i] = decorateStorage(st)
	}
	return out
}

// decorateStorageContent fills SizeHuman derived from raw byte counts.
func decorateStorageContent(items []domain.StorageContentItem) []domain.StorageContentItem {
	out := make([]domain.StorageContentItem, len(items))
	for i, item := range items {
		item.SizeHuman = formatBytes(item.Size)
		out[i] = item
	}
	return out
}

// decorateNodeResults rebuilds an aggregated node output with decorated data.
func decorateNodeResults(out *output.MultiProfileOutput[[]domain.Node]) {
	for i := range out.Results {
		out.Results[i].Data = decorateNodes(out.Results[i].Data)
	}
}

// decorateVMResults rebuilds an aggregated VM output with decorated data.
func decorateVMResults(out *output.MultiProfileOutput[[]domain.VM]) {
	for i := range out.Results {
		out.Results[i].Data = decorateVMs(out.Results[i].Data)
	}
}

// decorateContainerResults rebuilds an aggregated container output with
// decorated data.
func decorateContainerResults(out *output.MultiProfileOutput[[]domain.Container]) {
	for i := range out.Results {
		out.Results[i].Data = decorateContainers(out.Results[i].Data)
	}
}
