package cli

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/output"
	"github.com/geoffmcc/nodex/internal/redact"
)

const (
	statusTaskWarningLogLimit   = 100
	statusTaskWarningCacheLimit = 128
	statusTaskWarningQueryLimit = 5
)

type statusTaskWarningCache struct {
	details map[string]string
	seen    map[string]bool
	order   []string
}

func newStatusTaskWarningCache() *statusTaskWarningCache {
	return &statusTaskWarningCache{details: map[string]string{}, seen: map[string]bool{}}
}

func writeStatusGuestCounts(w io.Writer, label, state string, running, stopped, unknown int) {
	if state != "" && state != "available" {
		fmt.Fprintf(w, "%s: %s\n", label, state)
		return
	}
	fmt.Fprintf(w, "%s: %d running, %d stopped", label, running, stopped)
	if unknown > 0 {
		fmt.Fprintf(w, ", %d unknown/other", unknown)
	}
	fmt.Fprintln(w)
}

// Tasks are an explicitly bounded sample, not a cluster-wide count. Limit the
// number of node queries and retained results; never call task details/logs.
func collectStatusTasks(ctx context.Context, prov domain.Provider, snapshot *statusOverview) {
	collectStatusTasksWithWarnings(ctx, prov, snapshot, nil)
}

func collectStatusTasksWithWarnings(ctx context.Context, prov domain.Provider, snapshot *statusOverview, warningCache *statusTaskWarningCache) {
	snapshot.Sections["tasks"] = "unsupported"
	tp, ok := prov.(domain.TaskInspector)
	if !ok {
		return
	}
	snapshot.Sections["tasks"] = "available"
	if snapshot.Sections["nodes"] != "available" {
		snapshot.Sections["tasks"] = "unavailable"
		return
	}
	nodes := append([]statusNode(nil), snapshot.NodesDetail...)
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Name < nodes[j].Name })
	if len(nodes) > 8 {
		nodes = nodes[:8]
		snapshot.Warnings = append(snapshot.Warnings, "tasks: sampled first 8 nodes")
	}
	for _, node := range nodes {
		if node.Status != "online" || ctx.Err() != nil {
			snapshot.Sections["tasks"] = "unavailable"
			continue
		}
		tasks, err := tp.Tasks(ctx, node.Name)
		if err != nil {
			snapshot.Sections["tasks"] = "unavailable"
			continue
		}
		tasks = append([]domain.Task(nil), tasks...)
		sort.SliceStable(tasks, func(i, j int) bool {
			if (tasks[i].State == "running") != (tasks[j].State == "running") {
				return tasks[i].State == "running"
			}
			return tasks[i].StartTime > tasks[j].StartTime
		})
		if len(tasks) > 5 {
			tasks = tasks[:5]
		}
		snapshot.RecentTasks = append(snapshot.RecentTasks, tasks...)
	}
	if warningCache != nil {
		collectStatusTaskWarningDetails(ctx, prov, snapshot, warningCache)
	}
}

func collectStatusTaskWarningDetails(ctx context.Context, prov domain.Provider, snapshot *statusOverview, cache *statusTaskWarningCache) {
	logInspector, ok := prov.(domain.TaskLogInspector)
	if !ok || cache == nil {
		return
	}
	if snapshot.TaskWarningDetails == nil {
		snapshot.TaskWarningDetails = map[string]string{}
	}
	queries := 0
	for _, task := range snapshot.RecentTasks {
		if !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(task.Status)), "WARNINGS:") || task.UPID == "" {
			continue
		}
		if detail, found := cache.details[task.UPID]; found {
			snapshot.TaskWarningDetails[task.UPID] = detail
			continue
		}
		if cache.seen[task.UPID] || queries >= statusTaskWarningQueryLimit || ctx.Err() != nil {
			continue
		}
		cache.seen[task.UPID] = true
		cache.order = append(cache.order, task.UPID)
		queries++
		lines, err := logInspector.TaskLog(ctx, task.Node, task.UPID, statusTaskWarningLogLimit)
		if err != nil {
			continue
		}
		detail := summarizeTaskWarningLines(lines)
		if detail == "" {
			continue
		}
		cache.details[task.UPID] = detail
		snapshot.TaskWarningDetails[task.UPID] = detail
	}
	for len(cache.order) > statusTaskWarningCacheLimit {
		oldest := cache.order[0]
		cache.order = cache.order[1:]
		delete(cache.details, oldest)
		delete(cache.seen, oldest)
	}
}

func summarizeTaskWarningLines(lines []domain.TaskLogLine) string {
	var warnings []string
	for _, line := range lines {
		text := strings.TrimSpace(line.Text)
		upper := strings.ToUpper(text)
		if !strings.HasPrefix(upper, "WARN:") && !strings.HasPrefix(upper, "WARNING:") && !strings.HasPrefix(upper, "WARNING ") {
			continue
		}
		text = redact.String(output.SanitizeTerminal(text))
		text = strings.Join(strings.Fields(text), " ")
		if text == "" {
			continue
		}
		if len(text) > 180 {
			text = text[:177] + "..."
		}
		warnings = append(warnings, text)
		if len(warnings) == 2 {
			break
		}
	}
	return strings.Join(warnings, "; ")
}

func statusHealth(s *statusOverview) string {
	available, unavailable, unsupported := 0, 0, 0
	for _, section := range []string{"nodes", "vms", "containers", "storage", "cluster", "quorum"} {
		switch s.Sections[section] {
		case "available":
			available++
		case "unavailable":
			unavailable++
		default:
			unsupported++
		}
	}
	if available == 0 && unavailable > 0 {
		return "unreachable"
	}
	if unavailable > 0 {
		return "degraded"
	}
	for _, n := range s.NodesDetail {
		if n.Status != "online" {
			return "degraded"
		}
	}
	if s.QuorumKnown && s.Quorum == 0 {
		return "degraded"
	}
	for _, entry := range s.Storage {
		if !statusStorageMetricsAvailable(entry) {
			return "degraded"
		}
	}
	if s.StorageCapacity != nil {
		for _, g := range s.StorageCapacity.Groups {
			if g.Metrics == nil {
				return "degraded"
			}
		}
	}
	if s.Sections["tasks"] == "unavailable" || (!s.Standalone && s.Sections["ha"] == "unavailable") {
		return "degraded"
	}
	if unsupported > 0 || s.Nodes == 0 || s.VMsUnknown > 0 || s.CTsUnknown > 0 {
		return "unknown"
	}
	if !s.Standalone && (s.HA == nil || !s.HA.QuorumKnown) {
		return "unknown"
	}
	if !s.Standalone && s.HA.Quorum == 0 {
		return "degraded"
	}
	return "healthy"
}

// A successful refresh can report degraded health (e.g. an offline node).
// Success means the core queries and supported optional queries completed,
// not that the infrastructure is green. Standalone hosts do not require HA.
func statusRefreshComplete(s *statusOverview) bool {
	for _, section := range []string{"nodes", "vms", "containers", "storage", "cluster", "quorum"} {
		if s.Sections[section] != "available" {
			return false
		}
	}
	if s.Sections["tasks"] == "unavailable" || (!s.Standalone && s.Sections["ha"] == "unavailable") {
		return false
	}
	return true
}
