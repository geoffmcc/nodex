// Package backuphealth evaluates the combined health of a Proxmox VE +
// Proxmox Backup Server environment: provider reachability, datastore
// availability and capacity, running and recently failed backup-chain
// tasks, and per-guest backup coverage, age, and verification state.
//
// The service depends only on domain.Provider and the optional capability
// interfaces in internal/domain — never on concrete provider clients. Checks
// degrade honestly: data that cannot be retrieved yields StatusUnknown (and
// marks the result a partial failure), never StatusHealthy.
package backuphealth

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/geoffmcc/nodex/internal/domain"
)

// Status classifies a check outcome. Severity ordering (least to most
// severe): healthy < unsupported < unknown < warning < blocked. A result is
// never healthier than its least healthy required check.
type Status string

const (
	StatusHealthy     Status = "healthy"
	StatusUnsupported Status = "unsupported"
	StatusUnknown     Status = "unknown"
	StatusWarning     Status = "warning"
	StatusBlocked     Status = "blocked"
)

var statusSeverity = map[Status]int{
	StatusHealthy:     0,
	StatusUnsupported: 1,
	StatusUnknown:     2,
	StatusWarning:     3,
	StatusBlocked:     4,
}

// worse returns the more severe of two statuses.
func worse(a, b Status) Status {
	if statusSeverity[b] > statusSeverity[a] {
		return b
	}
	return a
}

// failedTaskWindow bounds the "recent failed tasks" check.
const failedTaskWindow = 24 * time.Hour

// Thresholds hold the environment's evaluation thresholds. All fields must
// be positive; use config defaults for unset values before building.
type Thresholds struct {
	BackupMaxAge          time.Duration
	VerifyMaxAge          time.Duration
	DatastoreWarnPercent  int
	DatastoreBlockPercent int
}

// Request describes one environment evaluation.
type Request struct {
	Environment string
	Thresholds  Thresholds

	// Namespaces are the PBS namespaces searched for guest backups; empty
	// means the root namespace only.
	Namespaces []string

	// ExcludeGuests lists VMIDs exempt from coverage checks.
	ExcludeGuests []int

	// IncludeGuests enables the per-guest backup coverage scan (the
	// difference between `environment health` and `environment
	// backup-health`).
	IncludeGuests bool
}

// Check is one named health check outcome.
type Check struct {
	Name   string `json:"name" yaml:"name"`
	Status Status `json:"status" yaml:"status"`
	Detail string `json:"detail,omitempty" yaml:"detail,omitempty"`
}

// GuestCoverage is the backup coverage evaluation for one protected guest.
type GuestCoverage struct {
	VMID         int    `json:"vmid" yaml:"vmid"`
	Name         string `json:"name" yaml:"name"`
	Type         string `json:"type" yaml:"type"` // vm | ct
	Node         string `json:"node" yaml:"node"`
	Status       Status `json:"status" yaml:"status"`
	NewestBackup int64  `json:"newest_backup,omitempty" yaml:"newest_backup,omitempty"`
	AgeHours     int64  `json:"age_hours,omitempty" yaml:"age_hours,omitempty"`
	Datastore    string `json:"datastore,omitempty" yaml:"datastore,omitempty"`
	Namespace    string `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	Verification string `json:"verification,omitempty" yaml:"verification,omitempty"` // ok | failed | none
	Detail       string `json:"detail,omitempty" yaml:"detail,omitempty"`
}

// Result is the full evaluation outcome.
type Result struct {
	Environment     string          `json:"environment" yaml:"environment"`
	CheckedAt       int64           `json:"checked_at" yaml:"checked_at"`
	Overall         Status          `json:"overall" yaml:"overall"`
	MaintenanceSafe bool            `json:"maintenance_safe" yaml:"maintenance_safe"`
	Blockers        []string        `json:"blockers,omitempty" yaml:"blockers,omitempty"`
	Checks          []Check         `json:"checks" yaml:"checks"`
	Guests          []GuestCoverage `json:"guests,omitempty" yaml:"guests,omitempty"`
	PartialFailure  bool            `json:"partial_failure" yaml:"partial_failure"`
	Errors          []string        `json:"errors,omitempty" yaml:"errors,omitempty"`
}

// BackupHealthReader is the capability boundary consumed by callers.
type BackupHealthReader interface {
	CheckEnvironmentBackupHealth(ctx context.Context, req Request) (*Result, error)
}

// Service implements BackupHealthReader over abstract providers. Either
// provider may be nil when the environment does not configure it; affected
// checks report StatusUnsupported.
type Service struct {
	PVE domain.Provider
	PBS domain.Provider

	// Now is injectable for deterministic tests; defaults to time.Now.
	Now func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// CheckEnvironmentBackupHealth evaluates the environment. It returns an
// error only for invalid requests; provider failures are recorded in the
// Result as unknown checks and partial failure.
func (s *Service) CheckEnvironmentBackupHealth(ctx context.Context, req Request) (*Result, error) {
	if req.Environment == "" {
		return nil, fmt.Errorf("environment name is required")
	}
	t := req.Thresholds
	if t.BackupMaxAge <= 0 || t.VerifyMaxAge <= 0 || t.DatastoreWarnPercent <= 0 || t.DatastoreBlockPercent <= 0 {
		return nil, fmt.Errorf("thresholds must be fully populated")
	}

	now := s.now()
	res := &Result{
		Environment: req.Environment,
		CheckedAt:   now.Unix(),
		Overall:     StatusHealthy,
	}
	maintenanceUnsafe := func(reason string) {
		res.Blockers = append(res.Blockers, reason)
	}
	record := func(c Check) {
		res.Checks = append(res.Checks, c)
		res.Overall = worse(res.Overall, c.Status)
	}
	retrievalError := func(name string, err error) Check {
		res.PartialFailure = true
		res.Errors = append(res.Errors, fmt.Sprintf("%s: %v", name, err))
		return Check{Name: name, Status: StatusUnknown, Detail: "could not retrieve: " + err.Error()}
	}

	// --- PVE reachability ---
	pveUp := false
	if s.PVE == nil {
		record(Check{Name: "pve_reachable", Status: StatusUnsupported, Detail: "no pve_profile configured"})
	} else if err := s.PVE.Health(ctx); err != nil {
		record(Check{Name: "pve_reachable", Status: StatusBlocked, Detail: err.Error()})
		maintenanceUnsafe("PVE is unreachable")
		res.PartialFailure = true
		res.Errors = append(res.Errors, fmt.Sprintf("pve_reachable: %v", err))
	} else {
		pveUp = true
		record(Check{Name: "pve_reachable", Status: StatusHealthy})
	}

	// --- PBS reachability ---
	pbsUp := false
	if s.PBS == nil {
		record(Check{Name: "pbs_reachable", Status: StatusUnsupported, Detail: "no pbs_profile configured"})
	} else if err := s.PBS.Health(ctx); err != nil {
		record(Check{Name: "pbs_reachable", Status: StatusBlocked, Detail: err.Error()})
		maintenanceUnsafe("PBS is unreachable")
		res.PartialFailure = true
		res.Errors = append(res.Errors, fmt.Sprintf("pbs_reachable: %v", err))
	} else {
		pbsUp = true
		record(Check{Name: "pbs_reachable", Status: StatusHealthy})
	}

	// --- Datastores ---
	var datastores []domain.PBSDatastoreUsage
	if pbsUp {
		if ds, ok := s.PBS.(domain.PBSDatastoreInspector); ok {
			usages, err := ds.PBSDatastoreUsages(ctx)
			if err != nil {
				c := retrievalError("pbs_datastores", err)
				record(c)
				maintenanceUnsafe("datastore state unknown")
			} else {
				datastores = usages
				record(s.checkDatastores(usages, t, maintenanceUnsafe))
			}
		} else {
			record(Check{Name: "pbs_datastores", Status: StatusUnsupported, Detail: "provider does not expose datastores"})
		}
	} else if s.PBS != nil {
		record(Check{Name: "pbs_datastores", Status: StatusUnknown, Detail: "PBS unreachable"})
	}

	// --- Active backup-chain tasks ---
	if pbsUp {
		if ti, ok := s.PBS.(domain.PBSTaskInspector); ok {
			running, err := ti.PBSTasks(ctx, domain.PBSTaskFilter{Running: true})
			if err != nil {
				c := retrievalError("pbs_active_tasks", err)
				record(c)
				maintenanceUnsafe("active-task state unknown")
			} else {
				record(checkActiveTasks(running, maintenanceUnsafe))
			}
		} else {
			record(Check{Name: "pbs_active_tasks", Status: StatusUnsupported, Detail: "provider does not expose tasks"})
			maintenanceUnsafe("active-task state unsupported")
		}
	} else if s.PBS != nil {
		record(Check{Name: "pbs_active_tasks", Status: StatusUnknown, Detail: "PBS unreachable"})
		maintenanceUnsafe("active-task state unknown")
	}

	// --- Recent failed PBS tasks ---
	if pbsUp {
		if ti, ok := s.PBS.(domain.PBSTaskInspector); ok {
			failed, err := ti.PBSTasks(ctx, domain.PBSTaskFilter{
				Errors: true,
				Since:  now.Add(-failedTaskWindow).Unix(),
			})
			if err != nil {
				record(retrievalError("pbs_failed_tasks", err))
			} else if len(failed) > 0 {
				record(Check{
					Name:   "pbs_failed_tasks",
					Status: StatusWarning,
					Detail: fmt.Sprintf("%d failed PBS task(s) in the last %s (newest: %s)", len(failed), failedTaskWindow, newestUPID(failed)),
				})
			} else {
				record(Check{Name: "pbs_failed_tasks", Status: StatusHealthy})
			}
		}
	}

	// --- Recent failed PVE vzdump tasks ---
	if pveUp {
		record(s.checkPVEBackupTasks(ctx, now, retrievalError))
	}

	// --- Guest coverage ---
	if req.IncludeGuests {
		s.checkGuestCoverage(ctx, req, now, datastores, res, record, maintenanceUnsafe, retrievalError)
	}

	// Maintenance safety: any blocker, or any non-healthy overall state
	// beyond pure warnings that don't affect safety, blocks maintenance.
	res.MaintenanceSafe = len(res.Blockers) == 0 && statusSeverity[res.Overall] < statusSeverity[StatusBlocked]
	sort.Strings(res.Blockers)
	return res, nil
}

// checkDatastores evaluates availability and capacity.
func (s *Service) checkDatastores(usages []domain.PBSDatastoreUsage, t Thresholds, unsafe func(string)) Check {
	if len(usages) == 0 {
		unsafe("no PBS datastores found")
		return Check{Name: "pbs_datastores", Status: StatusBlocked, Detail: "no datastores found"}
	}
	status := StatusHealthy
	var details []string
	for _, u := range usages {
		switch {
		case u.Error != "" || (u.MountStatus != "" && u.MountStatus != "ok"):
			status = worse(status, StatusBlocked)
			detail := u.Error
			if detail == "" {
				detail = "mount status " + u.MountStatus
			}
			details = append(details, fmt.Sprintf("%s: unavailable (%s)", u.Store, detail))
			unsafe(fmt.Sprintf("datastore %q unavailable", u.Store))
		case u.Total > 0:
			usedPct := int(u.Used * 100 / u.Total)
			if usedPct >= t.DatastoreBlockPercent {
				status = worse(status, StatusBlocked)
				details = append(details, fmt.Sprintf("%s: %d%% used (block threshold %d%%)", u.Store, usedPct, t.DatastoreBlockPercent))
				unsafe(fmt.Sprintf("datastore %q at %d%% capacity", u.Store, usedPct))
			} else if usedPct >= t.DatastoreWarnPercent {
				status = worse(status, StatusWarning)
				details = append(details, fmt.Sprintf("%s: %d%% used (warn threshold %d%%)", u.Store, usedPct, t.DatastoreWarnPercent))
			}
		}
	}
	return Check{Name: "pbs_datastores", Status: status, Detail: strings.Join(details, "; ")}
}

// backupChainWorkerTypes are the PBS worker types relevant to maintenance
// conflict awareness.
var backupChainWorkerTypes = map[string]bool{
	"backup":             true,
	"garbage_collection": true,
	"prune":              true,
	"prunejob":           true,
	"sync":               true,
	"syncjob":            true,
	"verify":             true,
	"verificationjob":    true,
	"verify_group":       true,
	"verify_snapshot":    true,
}

func checkActiveTasks(running []domain.PBSTask, unsafe func(string)) Check {
	var active []string
	for _, t := range running {
		if backupChainWorkerTypes[t.WorkerType] {
			active = append(active, fmt.Sprintf("%s(%s)", t.WorkerType, t.WorkerID))
		}
	}
	if len(active) == 0 {
		return Check{Name: "pbs_active_tasks", Status: StatusHealthy}
	}
	unsafe(fmt.Sprintf("%d active PBS backup-chain task(s): %s", len(active), strings.Join(active, ", ")))
	// Running maintenance is not unhealthy by itself; it only defers new
	// maintenance.
	return Check{
		Name:   "pbs_active_tasks",
		Status: StatusHealthy,
		Detail: "active: " + strings.Join(active, ", "),
	}
}

func newestUPID(tasks []domain.PBSTask) string {
	newest := ""
	var newestTime int64
	for _, t := range tasks {
		if t.StartTime >= newestTime {
			newestTime = t.StartTime
			newest = t.UPID
		}
	}
	return newest
}

// checkPVEBackupTasks scans each PVE node's recent tasks for failed vzdump
// runs inside the failed-task window.
func (s *Service) checkPVEBackupTasks(ctx context.Context, now time.Time, retrievalError func(string, error) Check) Check {
	ni, okNodes := s.PVE.(domain.NodeInspector)
	ti, okTasks := s.PVE.(domain.TaskInspector)
	if !okNodes || !okTasks {
		return Check{Name: "pve_failed_backup_tasks", Status: StatusUnsupported, Detail: "provider does not expose node tasks"}
	}
	nodes, err := ni.Nodes(ctx)
	if err != nil {
		return retrievalError("pve_failed_backup_tasks", err)
	}
	cutoff := now.Add(-failedTaskWindow).Unix()
	var failures []string
	for _, n := range nodes {
		tasks, err := ti.Tasks(ctx, n.Name)
		if err != nil {
			return retrievalError("pve_failed_backup_tasks", err)
		}
		for _, t := range tasks {
			if t.Type != "vzdump" {
				continue
			}
			if t.EndTime > 0 && int64(t.EndTime) < cutoff {
				continue
			}
			status := strings.ToUpper(t.Status)
			if status != "" && status != "OK" && status != "RUNNING" {
				failures = append(failures, fmt.Sprintf("%s on %s (%s)", t.UPID, n.Name, t.Status))
			}
		}
	}
	if len(failures) > 0 {
		return Check{
			Name:   "pve_failed_backup_tasks",
			Status: StatusWarning,
			Detail: fmt.Sprintf("%d failed vzdump task(s) in the last %s: %s", len(failures), failedTaskWindow, strings.Join(failures, "; ")),
		}
	}
	return Check{Name: "pve_failed_backup_tasks", Status: StatusHealthy}
}

// snapshotKey indexes newest snapshots by guest.
type snapshotKey struct {
	backupType string // vm | ct
	backupID   string
}

// checkGuestCoverage lists protected guests from PVE and finds each one's
// newest PBS backup across the environment's datastores and namespaces.
func (s *Service) checkGuestCoverage(
	ctx context.Context,
	req Request,
	now time.Time,
	datastores []domain.PBSDatastoreUsage,
	res *Result,
	record func(Check),
	unsafe func(string),
	retrievalError func(string, error) Check,
) {
	if s.PVE == nil || s.PBS == nil {
		record(Check{Name: "guest_backup_coverage", Status: StatusUnsupported, Detail: "requires both pve_profile and pbs_profile"})
		return
	}
	vi, okVM := s.PVE.(domain.VMInspector)
	ci, okCT := s.PVE.(domain.ContainerInspector)
	si, okSnap := s.PBS.(domain.PBSSnapshotInspector)
	if !okVM || !okCT || !okSnap {
		record(Check{Name: "guest_backup_coverage", Status: StatusUnsupported, Detail: "providers do not expose guests or snapshots"})
		return
	}

	vms, err := vi.VMs(ctx)
	if err != nil {
		record(retrievalError("guest_backup_coverage", err))
		unsafe("guest list unknown")
		return
	}
	cts, err := ci.Containers(ctx)
	if err != nil {
		record(retrievalError("guest_backup_coverage", err))
		unsafe("guest list unknown")
		return
	}

	// Index the newest snapshot per (type, vmid) across datastores and
	// namespaces. One listing per datastore+namespace pair.
	namespaces := req.Namespaces
	if len(namespaces) == 0 {
		namespaces = []string{""}
	}
	type located struct {
		snap      domain.PBSSnapshot
		datastore string
		namespace string
	}
	newest := map[snapshotKey]located{}
	scanFailed := false
	for _, ds := range datastores {
		if ds.Error != "" || (ds.MountStatus != "" && ds.MountStatus != "ok") {
			continue // unavailable datastores already reported
		}
		for _, ns := range namespaces {
			snaps, err := si.PBSSnapshots(ctx, ds.Store, domain.PBSSnapshotFilter{Namespace: ns})
			if err != nil {
				res.PartialFailure = true
				res.Errors = append(res.Errors, fmt.Sprintf("snapshots %s ns %q: %v", ds.Store, ns, err))
				scanFailed = true
				continue
			}
			for _, snap := range snaps {
				key := snapshotKey{backupType: snap.BackupType, backupID: snap.BackupID}
				if cur, ok := newest[key]; !ok || snap.BackupTime > cur.snap.BackupTime {
					newest[key] = located{snap: snap, datastore: ds.Store, namespace: ns}
				}
			}
		}
	}

	excluded := map[int]bool{}
	for _, vmid := range req.ExcludeGuests {
		excluded[vmid] = true
	}

	coverage := StatusHealthy
	evaluate := func(guestType, id, name, node string) {
		vmid := parseVMID(id)
		if vmid <= 0 || excluded[vmid] {
			return
		}
		g := GuestCoverage{VMID: vmid, Name: name, Type: guestType, Node: node, Status: StatusHealthy, Verification: "none"}
		loc, ok := newest[snapshotKey{backupType: guestType, backupID: strconv.Itoa(vmid)}]
		if !ok {
			if scanFailed {
				g.Status = StatusUnknown
				g.Detail = "no backup found, and one or more snapshot listings failed"
			} else {
				g.Status = StatusBlocked
				g.Detail = "no backup found in any searched datastore/namespace"
				unsafe(fmt.Sprintf("guest %d (%s) has no backup", vmid, name))
			}
			coverage = worse(coverage, g.Status)
			res.Guests = append(res.Guests, g)
			return
		}
		g.NewestBackup = loc.snap.BackupTime
		g.Datastore = loc.datastore
		g.Namespace = loc.namespace
		age := now.Sub(time.Unix(loc.snap.BackupTime, 0))
		g.AgeHours = int64(age.Hours())
		if loc.snap.Verification != nil {
			g.Verification = loc.snap.Verification.State
		}

		if age > req.Thresholds.BackupMaxAge {
			g.Status = StatusWarning
			g.Detail = fmt.Sprintf("newest backup is %dh old (threshold %dh)", g.AgeHours, int64(req.Thresholds.BackupMaxAge.Hours()))
			unsafe(fmt.Sprintf("guest %d (%s) backup is stale (%dh)", vmid, name, g.AgeHours))
		}
		switch g.Verification {
		case "failed":
			g.Status = worse(g.Status, StatusWarning)
			g.Detail = strings.TrimSpace(g.Detail + "; verification failed")
			unsafe(fmt.Sprintf("guest %d (%s) newest backup failed verification", vmid, name))
		case "none":
			if age > req.Thresholds.VerifyMaxAge {
				g.Status = worse(g.Status, StatusWarning)
				g.Detail = strings.TrimSpace(g.Detail + "; unverified beyond verify threshold")
			}
		}
		coverage = worse(coverage, g.Status)
		res.Guests = append(res.Guests, g)
	}

	for _, vm := range vms {
		evaluate("vm", vm.ID, vm.Name, vm.Node)
	}
	for _, ct := range cts {
		evaluate("ct", ct.ID, ct.Name, ct.Node)
	}

	sort.Slice(res.Guests, func(i, j int) bool { return res.Guests[i].VMID < res.Guests[j].VMID })

	detail := fmt.Sprintf("%d protected guest(s) evaluated", len(res.Guests))
	if scanFailed {
		coverage = worse(coverage, StatusUnknown)
		detail += "; some snapshot listings failed"
	}
	record(Check{Name: "guest_backup_coverage", Status: coverage, Detail: detail})
}

// parseVMID extracts the numeric VMID from a guest ID like "node/100".
func parseVMID(id string) int {
	if idx := strings.LastIndex(id, "/"); idx >= 0 {
		id = id[idx+1:]
	}
	vmid, err := strconv.Atoi(id)
	if err != nil {
		return 0
	}
	return vmid
}
