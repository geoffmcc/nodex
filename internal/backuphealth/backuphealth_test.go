package backuphealth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/geoffmcc/nodex/internal/domain"
)

// fixedNow anchors all age calculations: 2026-07-17T12:00:00Z.
var fixedNow = time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)

func hoursAgo(h int) int64 { return fixedNow.Add(-time.Duration(h) * time.Hour).Unix() }

func defaultThresholds() Thresholds {
	return Thresholds{
		BackupMaxAge:          26 * time.Hour,
		VerifyMaxAge:          8 * 24 * time.Hour,
		DatastoreWarnPercent:  80,
		DatastoreBlockPercent: 95,
	}
}

// fakePVE implements domain.Provider + NodeInspector + VMInspector +
// ContainerInspector + TaskInspector.
type fakePVE struct {
	healthErr error
	vms       []domain.VM
	cts       []domain.Container
	tasks     []domain.Task
	tasksErr  error
}

func (f *fakePVE) Name() string                                               { return "proxmox" }
func (f *fakePVE) Version() string                                            { return "test" }
func (f *fakePVE) Connect(context.Context, string, *domain.Credentials) error { return nil }
func (f *fakePVE) Close() error                                               { return nil }
func (f *fakePVE) Health(context.Context) error                               { return f.healthErr }
func (f *fakePVE) Capabilities() []domain.Capability                          { return nil }
func (f *fakePVE) Nodes(context.Context) ([]domain.Node, error) {
	return []domain.Node{{Name: "pve1"}}, nil
}
func (f *fakePVE) VMs(context.Context) ([]domain.VM, error)               { return f.vms, nil }
func (f *fakePVE) Containers(context.Context) ([]domain.Container, error) { return f.cts, nil }
func (f *fakePVE) Tasks(_ context.Context, node string) ([]domain.Task, error) {
	return f.tasks, f.tasksErr
}
func (f *fakePVE) Task(context.Context, string, string) (*domain.Task, error) {
	return nil, errors.New("not implemented")
}
func (f *fakePVE) VMConfig(context.Context, string, int) (map[string]interface{}, error) {
	return nil, errors.New("not implemented")
}
func (f *fakePVE) ContainerConfig(context.Context, string, int) (map[string]interface{}, error) {
	return nil, errors.New("not implemented")
}

// fakePBS implements domain.Provider + PBSDatastoreInspector +
// PBSSnapshotInspector + PBSTaskInspector.
type fakePBS struct {
	healthErr    error
	usages       []domain.PBSDatastoreUsage
	usagesErr    error
	snapshots    map[string][]domain.PBSSnapshot // key: store + "|" + ns
	snapshotsErr error
	running      []domain.PBSTask
	failed       []domain.PBSTask
	tasksErr     error
}

func (f *fakePBS) Name() string                                               { return "pbs" }
func (f *fakePBS) Version() string                                            { return "test" }
func (f *fakePBS) Connect(context.Context, string, *domain.Credentials) error { return nil }
func (f *fakePBS) Close() error                                               { return nil }
func (f *fakePBS) Health(context.Context) error                               { return f.healthErr }
func (f *fakePBS) Capabilities() []domain.Capability                          { return nil }

func (f *fakePBS) PBSDatastores(context.Context) ([]domain.PBSDatastore, error) { return nil, nil }
func (f *fakePBS) PBSDatastore(context.Context, string) (*domain.PBSDatastore, error) {
	return nil, errors.New("not implemented")
}
func (f *fakePBS) PBSDatastoreStatus(context.Context, string) (*domain.PBSDatastoreStatus, error) {
	return nil, errors.New("not implemented")
}
func (f *fakePBS) PBSDatastoreUsages(context.Context) ([]domain.PBSDatastoreUsage, error) {
	return f.usages, f.usagesErr
}
func (f *fakePBS) PBSSnapshots(_ context.Context, store string, filter domain.PBSSnapshotFilter) ([]domain.PBSSnapshot, error) {
	if f.snapshotsErr != nil {
		return nil, f.snapshotsErr
	}
	return f.snapshots[store+"|"+filter.Namespace], nil
}
func (f *fakePBS) PBSTasks(_ context.Context, filter domain.PBSTaskFilter) ([]domain.PBSTask, error) {
	if f.tasksErr != nil {
		return nil, f.tasksErr
	}
	if filter.Running {
		return f.running, nil
	}
	if filter.Errors {
		return f.failed, nil
	}
	return nil, nil
}
func (f *fakePBS) PBSTaskStatus(context.Context, string) (*domain.PBSTaskStatus, error) {
	return nil, errors.New("not implemented")
}
func (f *fakePBS) PBSTaskLog(context.Context, string) ([]domain.PBSTaskLogLine, error) {
	return nil, errors.New("not implemented")
}

func healthyFakes() (*fakePVE, *fakePBS) {
	pve := &fakePVE{
		vms: []domain.VM{{ID: "pve1/100", Name: "web", Node: "pve1"}},
		cts: []domain.Container{{ID: "pve1/200", Name: "db", Node: "pve1"}},
	}
	pbs := &fakePBS{
		usages: []domain.PBSDatastoreUsage{
			{Store: "backups", Total: 1000, Used: 400, Avail: 600, MountStatus: "ok"},
		},
		snapshots: map[string][]domain.PBSSnapshot{
			"backups|": {
				{Store: "backups", BackupType: "vm", BackupID: "100", BackupTime: hoursAgo(5),
					Verification: &domain.PBSVerificationState{State: "ok"}},
				{Store: "backups", BackupType: "ct", BackupID: "200", BackupTime: hoursAgo(6),
					Verification: &domain.PBSVerificationState{State: "ok"}},
			},
		},
	}
	return pve, pbs
}

func run(t *testing.T, pve domain.Provider, pbs domain.Provider, mutate func(*Request)) *Result {
	t.Helper()
	svc := &Service{PVE: pve, PBS: pbs, Now: func() time.Time { return fixedNow }}
	req := Request{
		Environment:   "homelab",
		Thresholds:    defaultThresholds(),
		IncludeGuests: true,
	}
	if mutate != nil {
		mutate(&req)
	}
	res, err := svc.CheckEnvironmentBackupHealth(context.Background(), req)
	if err != nil {
		t.Fatalf("CheckEnvironmentBackupHealth: %v", err)
	}
	return res
}

func findCheck(t *testing.T, res *Result, name string) Check {
	t.Helper()
	for _, c := range res.Checks {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("check %q not found in %+v", name, res.Checks)
	return Check{}
}

func TestAllHealthy(t *testing.T) {
	pve, pbs := healthyFakes()
	res := run(t, pve, pbs, nil)
	if res.Overall != StatusHealthy {
		t.Errorf("overall = %s, want healthy (checks: %+v)", res.Overall, res.Checks)
	}
	if !res.MaintenanceSafe {
		t.Errorf("maintenance should be safe, blockers: %v", res.Blockers)
	}
	if res.PartialFailure {
		t.Error("no partial failure expected")
	}
	if len(res.Guests) != 2 {
		t.Fatalf("expected 2 guests, got %d", len(res.Guests))
	}
	for _, g := range res.Guests {
		if g.Status != StatusHealthy {
			t.Errorf("guest %d status = %s, want healthy (%s)", g.VMID, g.Status, g.Detail)
		}
		if g.Datastore != "backups" {
			t.Errorf("guest %d datastore = %q", g.VMID, g.Datastore)
		}
	}
}

func TestPBSDownNeverHealthy(t *testing.T) {
	pve, pbs := healthyFakes()
	pbs.healthErr = errors.New("connection refused")
	res := run(t, pve, pbs, nil)
	if res.Overall == StatusHealthy {
		t.Error("overall must not be healthy when PBS is down")
	}
	if res.MaintenanceSafe {
		t.Error("maintenance must not be safe when PBS is down")
	}
	if !res.PartialFailure {
		t.Error("PBS down is a partial failure")
	}
	if c := findCheck(t, res, "pbs_reachable"); c.Status != StatusBlocked {
		t.Errorf("pbs_reachable = %s, want blocked", c.Status)
	}
}

func TestPVEDownStillChecksPBS(t *testing.T) {
	pve, pbs := healthyFakes()
	pve.healthErr = errors.New("connection refused")
	res := run(t, pve, pbs, nil)
	if c := findCheck(t, res, "pbs_datastores"); c.Status != StatusHealthy {
		t.Errorf("pbs_datastores should still be evaluated, got %s", c.Status)
	}
	if res.MaintenanceSafe {
		t.Error("maintenance must not be safe when PVE is down")
	}
}

func TestStaleBackupWarns(t *testing.T) {
	pve, pbs := healthyFakes()
	pbs.snapshots["backups|"][0].BackupTime = hoursAgo(30) // vm 100 stale
	res := run(t, pve, pbs, nil)
	if res.Overall != StatusWarning {
		t.Errorf("overall = %s, want warning", res.Overall)
	}
	if res.MaintenanceSafe {
		t.Error("stale backup must block maintenance")
	}
	var vm100 GuestCoverage
	for _, g := range res.Guests {
		if g.VMID == 100 {
			vm100 = g
		}
	}
	if vm100.Status != StatusWarning || vm100.AgeHours != 30 {
		t.Errorf("vm100 = %+v, want warning at 30h", vm100)
	}
}

func TestMissingBackupBlocks(t *testing.T) {
	pve, pbs := healthyFakes()
	pbs.snapshots["backups|"] = pbs.snapshots["backups|"][:1] // drop ct 200
	res := run(t, pve, pbs, nil)
	if res.Overall != StatusBlocked {
		t.Errorf("overall = %s, want blocked", res.Overall)
	}
	if res.MaintenanceSafe {
		t.Error("missing backup must block maintenance")
	}
}

func TestExcludedGuestSkipped(t *testing.T) {
	pve, pbs := healthyFakes()
	pbs.snapshots["backups|"] = pbs.snapshots["backups|"][:1] // ct 200 has no backup
	res := run(t, pve, pbs, func(r *Request) { r.ExcludeGuests = []int{200} })
	if res.Overall != StatusHealthy {
		t.Errorf("overall = %s, want healthy with 200 excluded", res.Overall)
	}
	if len(res.Guests) != 1 {
		t.Errorf("expected 1 evaluated guest, got %d", len(res.Guests))
	}
}

func TestFailedVerificationWarns(t *testing.T) {
	pve, pbs := healthyFakes()
	pbs.snapshots["backups|"][0].Verification = &domain.PBSVerificationState{State: "failed"}
	res := run(t, pve, pbs, nil)
	if res.Overall != StatusWarning {
		t.Errorf("overall = %s, want warning", res.Overall)
	}
	if res.MaintenanceSafe {
		t.Error("failed verification must block maintenance")
	}
}

func TestUnverifiedRecentBackupHealthy(t *testing.T) {
	pve, pbs := healthyFakes()
	pbs.snapshots["backups|"][0].Verification = nil // recent, just unverified
	res := run(t, pve, pbs, nil)
	if res.Overall != StatusHealthy {
		t.Errorf("recent unverified backup should stay healthy, got %s", res.Overall)
	}
}

func TestUnverifiedOldBackupWarns(t *testing.T) {
	pve, pbs := healthyFakes()
	pbs.snapshots["backups|"][0].Verification = nil
	pbs.snapshots["backups|"][0].BackupTime = hoursAgo(9 * 24) // 9 days, beyond verify max age
	res := run(t, pve, pbs, nil)
	if res.Overall != StatusWarning {
		t.Errorf("overall = %s, want warning for unverified old backup", res.Overall)
	}
}

func TestDatastoreCapacityThresholds(t *testing.T) {
	pve, pbs := healthyFakes()
	pbs.usages[0].Used = 850 // 85% > warn 80
	res := run(t, pve, pbs, nil)
	if c := findCheck(t, res, "pbs_datastores"); c.Status != StatusWarning {
		t.Errorf("85%% used = %s, want warning", c.Status)
	}

	pbs.usages[0].Used = 960 // 96% > block 95
	res = run(t, pve, pbs, nil)
	if c := findCheck(t, res, "pbs_datastores"); c.Status != StatusBlocked {
		t.Errorf("96%% used = %s, want blocked", c.Status)
	}
	if res.MaintenanceSafe {
		t.Error("datastore above block threshold must block maintenance")
	}
}

func TestUnavailableDatastoreBlocks(t *testing.T) {
	pve, pbs := healthyFakes()
	pbs.usages = append(pbs.usages, domain.PBSDatastoreUsage{
		Store: "removable", MountStatus: "notmounted", Error: "not mounted",
	})
	res := run(t, pve, pbs, nil)
	if c := findCheck(t, res, "pbs_datastores"); c.Status != StatusBlocked {
		t.Errorf("unavailable datastore = %s, want blocked", c.Status)
	}
}

func TestActiveBackupTaskDefersMaintenance(t *testing.T) {
	pve, pbs := healthyFakes()
	pbs.running = []domain.PBSTask{{
		UPID: "UPID:x", WorkerType: "backup", WorkerID: "backups:vm/100", Status: "running",
	}}
	res := run(t, pve, pbs, nil)
	if res.Overall != StatusHealthy {
		t.Errorf("active task should not degrade health, got %s", res.Overall)
	}
	if res.MaintenanceSafe {
		t.Error("active backup task must defer maintenance")
	}
}

func TestFailedPBSTasksWarn(t *testing.T) {
	pve, pbs := healthyFakes()
	pbs.failed = []domain.PBSTask{{
		UPID: "UPID:failed", WorkerType: "garbage_collection", StartTime: hoursAgo(2), Status: "gc failed",
	}}
	res := run(t, pve, pbs, nil)
	if c := findCheck(t, res, "pbs_failed_tasks"); c.Status != StatusWarning {
		t.Errorf("failed tasks = %s, want warning", c.Status)
	}
}

func TestFailedPVEVzdumpWarns(t *testing.T) {
	pve, pbs := healthyFakes()
	pve.tasks = []domain.Task{
		{UPID: "UPID:pve1:vzdump-fail", Type: "vzdump", Status: "backup error", EndTime: int(hoursAgo(3))},
		{UPID: "UPID:pve1:vzdump-old", Type: "vzdump", Status: "some old error", EndTime: int(hoursAgo(48))},
		{UPID: "UPID:pve1:other", Type: "qmstart", Status: "task error", EndTime: int(hoursAgo(1))},
	}
	res := run(t, pve, pbs, nil)
	c := findCheck(t, res, "pve_failed_backup_tasks")
	if c.Status != StatusWarning {
		t.Errorf("pve_failed_backup_tasks = %s, want warning", c.Status)
	}
	if !strings.Contains(c.Detail, "1 failed vzdump") {
		t.Errorf("only the recent vzdump failure should count: %s", c.Detail)
	}
}

func TestSnapshotListingFailureNeverHealthy(t *testing.T) {
	pve, pbs := healthyFakes()
	pbs.snapshotsErr = errors.New("permission denied")
	res := run(t, pve, pbs, nil)
	if res.Overall == StatusHealthy {
		t.Error("overall must not be healthy when snapshot listings fail")
	}
	if !res.PartialFailure {
		t.Error("failed listing must set PartialFailure")
	}
	for _, g := range res.Guests {
		if g.Status == StatusBlocked {
			t.Errorf("guest %d should be unknown, not blocked, when listings failed", g.VMID)
		}
	}
}

func TestDatastoreUsageFailureBlocksMaintenance(t *testing.T) {
	pve, pbs := healthyFakes()
	pbs.usagesErr = errors.New("500 internal error")
	res := run(t, pve, pbs, nil)
	if res.Overall == StatusHealthy {
		t.Error("overall must not be healthy when datastore usage is unknown")
	}
	if res.MaintenanceSafe {
		t.Error("unknown datastore state must block maintenance")
	}
	if c := findCheck(t, res, "pbs_datastores"); c.Status != StatusUnknown {
		t.Errorf("pbs_datastores = %s, want unknown", c.Status)
	}
}

func TestPBSOnlyEnvironment(t *testing.T) {
	_, pbs := healthyFakes()
	res := run(t, nil, pbs, func(r *Request) { r.IncludeGuests = true })
	if c := findCheck(t, res, "pve_reachable"); c.Status != StatusUnsupported {
		t.Errorf("pve_reachable = %s, want unsupported", c.Status)
	}
	if c := findCheck(t, res, "guest_backup_coverage"); c.Status != StatusUnsupported {
		t.Errorf("guest coverage without PVE = %s, want unsupported", c.Status)
	}
	// Unsupported checks are not failures, but they keep the result from
	// claiming full health.
	if res.Overall != StatusUnsupported {
		t.Errorf("overall = %s, want unsupported", res.Overall)
	}
}

func TestNamespaceSearch(t *testing.T) {
	pve, pbs := healthyFakes()
	// Move ct 200's backup into the "prod" namespace.
	root := pbs.snapshots["backups|"]
	pbs.snapshots["backups|"] = root[:1]
	ct := root[1]
	ct.Namespace = "prod"
	pbs.snapshots["backups|prod"] = []domain.PBSSnapshot{ct}

	// Without the namespace configured, ct 200 appears unprotected.
	res := run(t, pve, pbs, nil)
	if res.Overall != StatusBlocked {
		t.Errorf("overall without namespace = %s, want blocked", res.Overall)
	}

	// With the namespace configured, coverage is complete.
	res = run(t, pve, pbs, func(r *Request) { r.Namespaces = []string{"", "prod"} })
	if res.Overall != StatusHealthy {
		t.Errorf("overall with namespace = %s, want healthy", res.Overall)
	}
}

func TestInvalidRequests(t *testing.T) {
	svc := &Service{}
	if _, err := svc.CheckEnvironmentBackupHealth(context.Background(), Request{}); err == nil {
		t.Error("empty environment must be rejected")
	}
	if _, err := svc.CheckEnvironmentBackupHealth(context.Background(), Request{Environment: "x"}); err == nil {
		t.Error("empty thresholds must be rejected")
	}
}

func TestHealthOnlySkipsGuests(t *testing.T) {
	pve, pbs := healthyFakes()
	res := run(t, pve, pbs, func(r *Request) { r.IncludeGuests = false })
	if len(res.Guests) != 0 {
		t.Errorf("health-only mode must not evaluate guests, got %d", len(res.Guests))
	}
	if res.Overall != StatusHealthy {
		t.Errorf("overall = %s, want healthy", res.Overall)
	}
}
