package domain

import "context"

// PBS domain models and capability interfaces. These use native Proxmox
// Backup Server terminology (datastores, backup groups/snapshots,
// namespaces, verify/prune/sync jobs, garbage collection) rather than
// forcing PBS resources into Proxmox VE shapes.

// PBS capability constants.
const (
	CapabilityPBSSystem     Capability = "pbs_system"
	CapabilityPBSDatastores Capability = "pbs_datastores"
	CapabilityPBSSnapshots  Capability = "pbs_snapshots"
	CapabilityPBSTasks      Capability = "pbs_tasks"
	CapabilityPBSJobs       Capability = "pbs_jobs"
	CapabilityPBSGC         Capability = "pbs_gc"
)

// PBSVersionInfo is the PBS server version (GET /version).
type PBSVersionInfo struct {
	Version string `json:"version" yaml:"version"`
	Release string `json:"release" yaml:"release"`
	RepoID  string `json:"repoid" yaml:"repoid"`
}

// PBSNodeStatus is the PBS host status (GET /nodes/{node}/status).
type PBSNodeStatus struct {
	CPU           float64   `json:"cpu" yaml:"cpu"`
	Wait          float64   `json:"wait" yaml:"wait"`
	Uptime        int64     `json:"uptime" yaml:"uptime"`
	LoadAvg       []float64 `json:"loadavg" yaml:"loadavg"`
	KernelVersion string    `json:"kversion" yaml:"kversion"`
	CPUModel      string    `json:"cpu_model,omitempty" yaml:"cpu_model,omitempty"`
	CPUs          int64     `json:"cpus,omitempty" yaml:"cpus,omitempty"`
	MemoryTotal   int64     `json:"memory_total" yaml:"memory_total"`
	MemoryUsed    int64     `json:"memory_used" yaml:"memory_used"`
	MemoryFree    int64     `json:"memory_free" yaml:"memory_free"`
	SwapTotal     int64     `json:"swap_total" yaml:"swap_total"`
	SwapUsed      int64     `json:"swap_used" yaml:"swap_used"`
	RootTotal     int64     `json:"root_total" yaml:"root_total"`
	RootUsed      int64     `json:"root_used" yaml:"root_used"`
	RootAvail     int64     `json:"root_avail" yaml:"root_avail"`
	BootMode      string    `json:"boot_mode,omitempty" yaml:"boot_mode,omitempty"`
}

// PBSDatastore is a datastore configuration entry (GET /config/datastore).
type PBSDatastore struct {
	Name            string `json:"name" yaml:"name"`
	Path            string `json:"path" yaml:"path"`
	Comment         string `json:"comment,omitempty" yaml:"comment,omitempty"`
	GCSchedule      string `json:"gc_schedule,omitempty" yaml:"gc_schedule,omitempty"`
	PruneSchedule   string `json:"prune_schedule,omitempty" yaml:"prune_schedule,omitempty"`
	KeepLast        int64  `json:"keep_last,omitempty" yaml:"keep_last,omitempty"`
	KeepHourly      int64  `json:"keep_hourly,omitempty" yaml:"keep_hourly,omitempty"`
	KeepDaily       int64  `json:"keep_daily,omitempty" yaml:"keep_daily,omitempty"`
	KeepWeekly      int64  `json:"keep_weekly,omitempty" yaml:"keep_weekly,omitempty"`
	KeepMonthly     int64  `json:"keep_monthly,omitempty" yaml:"keep_monthly,omitempty"`
	KeepYearly      int64  `json:"keep_yearly,omitempty" yaml:"keep_yearly,omitempty"`
	VerifyNew       bool   `json:"verify_new,omitempty" yaml:"verify_new,omitempty"`
	MaintenanceMode string `json:"maintenance_mode,omitempty" yaml:"maintenance_mode,omitempty"`
	Backend         string `json:"backend,omitempty" yaml:"backend,omitempty"`
}

// PBSDatastoreStatus is a datastore's usage/status
// (GET /admin/datastore/{store}/status).
type PBSDatastoreStatus struct {
	Store       string `json:"store" yaml:"store"`
	Total       int64  `json:"total" yaml:"total"`
	Used        int64  `json:"used" yaml:"used"`
	Avail       int64  `json:"avail" yaml:"avail"`
	BackendType string `json:"backend_type,omitempty" yaml:"backend_type,omitempty"`
}

// PBSDatastoreUsage is one entry of GET /status/datastore-usage.
type PBSDatastoreUsage struct {
	Store             string `json:"store" yaml:"store"`
	Total             int64  `json:"total" yaml:"total"`
	Used              int64  `json:"used" yaml:"used"`
	Avail             int64  `json:"avail" yaml:"avail"`
	MountStatus       string `json:"mount_status,omitempty" yaml:"mount_status,omitempty"`
	EstimatedFullDate int64  `json:"estimated_full_date,omitempty" yaml:"estimated_full_date,omitempty"`
	Error             string `json:"error,omitempty" yaml:"error,omitempty"`
}

// PBSVerificationState is a snapshot's verification result.
type PBSVerificationState struct {
	State string `json:"state" yaml:"state"`
	UPID  string `json:"upid" yaml:"upid"`
}

// PBSSnapshot is a backup snapshot (GET /admin/datastore/{store}/snapshots).
type PBSSnapshot struct {
	Store        string                `json:"store" yaml:"store"`
	Namespace    string                `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	BackupType   string                `json:"backup_type" yaml:"backup_type"`
	BackupID     string                `json:"backup_id" yaml:"backup_id"`
	BackupTime   int64                 `json:"backup_time" yaml:"backup_time"`
	Size         int64                 `json:"size,omitempty" yaml:"size,omitempty"`
	Owner        string                `json:"owner,omitempty" yaml:"owner,omitempty"`
	Protected    bool                  `json:"protected" yaml:"protected"`
	Comment      string                `json:"comment,omitempty" yaml:"comment,omitempty"`
	Fingerprint  string                `json:"fingerprint,omitempty" yaml:"fingerprint,omitempty"`
	Files        []string              `json:"files,omitempty" yaml:"files,omitempty"`
	Verification *PBSVerificationState `json:"verification,omitempty" yaml:"verification,omitempty"`
}

// PBSSnapshotFilter narrows a snapshot listing.
type PBSSnapshotFilter struct {
	Namespace  string
	BackupType string
	BackupID   string
}

// PBSTask is a task listing entry (GET /nodes/{node}/tasks).
type PBSTask struct {
	UPID       string `json:"upid" yaml:"upid"`
	Node       string `json:"node" yaml:"node"`
	WorkerType string `json:"worker_type" yaml:"worker_type"`
	WorkerID   string `json:"worker_id,omitempty" yaml:"worker_id,omitempty"`
	User       string `json:"user" yaml:"user"`
	StartTime  int64  `json:"starttime" yaml:"starttime"`
	EndTime    int64  `json:"endtime,omitempty" yaml:"endtime,omitempty"`
	Status     string `json:"status,omitempty" yaml:"status,omitempty"`
}

// PBSTaskFilter narrows a task listing.
type PBSTaskFilter struct {
	Running    bool
	Errors     bool
	Limit      int64
	Store      string
	TypeFilter string
	Since      int64
	Until      int64
}

// PBSTaskStatus is detailed task state (GET /nodes/{node}/tasks/{upid}/status).
type PBSTaskStatus struct {
	UPID       string `json:"upid" yaml:"upid"`
	Node       string `json:"node" yaml:"node"`
	PID        int64  `json:"pid" yaml:"pid"`
	WorkerType string `json:"worker_type" yaml:"worker_type"`
	WorkerID   string `json:"worker_id,omitempty" yaml:"worker_id,omitempty"`
	User       string `json:"user" yaml:"user"`
	StartTime  int64  `json:"starttime" yaml:"starttime"`
	EndTime    int64  `json:"endtime,omitempty" yaml:"endtime,omitempty"`
	Status     string `json:"status" yaml:"status"`
	ExitStatus string `json:"exitstatus,omitempty" yaml:"exitstatus,omitempty"`
}

// PBSTaskLogLine is one task log line (GET /nodes/{node}/tasks/{upid}/log).
type PBSTaskLogLine struct {
	LineNumber int64  `json:"n" yaml:"n"`
	Text       string `json:"t" yaml:"t"`
}

// PBSVerifyJob is a verification job (GET /config/verify).
type PBSVerifyJob struct {
	ID             string `json:"id" yaml:"id"`
	Store          string `json:"store" yaml:"store"`
	Namespace      string `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	Schedule       string `json:"schedule,omitempty" yaml:"schedule,omitempty"`
	Comment        string `json:"comment,omitempty" yaml:"comment,omitempty"`
	IgnoreVerified bool   `json:"ignore_verified,omitempty" yaml:"ignore_verified,omitempty"`
	OutdatedAfter  int64  `json:"outdated_after,omitempty" yaml:"outdated_after,omitempty"`
	MaxDepth       int64  `json:"max_depth,omitempty" yaml:"max_depth,omitempty"`
}

// PBSPruneJob is a prune job (GET /config/prune).
type PBSPruneJob struct {
	ID          string `json:"id" yaml:"id"`
	Store       string `json:"store" yaml:"store"`
	Namespace   string `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	Schedule    string `json:"schedule" yaml:"schedule"`
	Comment     string `json:"comment,omitempty" yaml:"comment,omitempty"`
	Disable     bool   `json:"disable,omitempty" yaml:"disable,omitempty"`
	KeepLast    int64  `json:"keep_last,omitempty" yaml:"keep_last,omitempty"`
	KeepHourly  int64  `json:"keep_hourly,omitempty" yaml:"keep_hourly,omitempty"`
	KeepDaily   int64  `json:"keep_daily,omitempty" yaml:"keep_daily,omitempty"`
	KeepWeekly  int64  `json:"keep_weekly,omitempty" yaml:"keep_weekly,omitempty"`
	KeepMonthly int64  `json:"keep_monthly,omitempty" yaml:"keep_monthly,omitempty"`
	KeepYearly  int64  `json:"keep_yearly,omitempty" yaml:"keep_yearly,omitempty"`
	MaxDepth    int64  `json:"max_depth,omitempty" yaml:"max_depth,omitempty"`
}

// PBSSyncJob is a sync job (GET /config/sync).
type PBSSyncJob struct {
	ID              string `json:"id" yaml:"id"`
	Store           string `json:"store" yaml:"store"`
	Namespace       string `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	Remote          string `json:"remote,omitempty" yaml:"remote,omitempty"`
	RemoteStore     string `json:"remote_store" yaml:"remote_store"`
	RemoteNamespace string `json:"remote_namespace,omitempty" yaml:"remote_namespace,omitempty"`
	Owner           string `json:"owner,omitempty" yaml:"owner,omitempty"`
	Schedule        string `json:"schedule,omitempty" yaml:"schedule,omitempty"`
	Comment         string `json:"comment,omitempty" yaml:"comment,omitempty"`
	RemoveVanished  bool   `json:"remove_vanished,omitempty" yaml:"remove_vanished,omitempty"`
	SyncDirection   string `json:"sync_direction,omitempty" yaml:"sync_direction,omitempty"`
}

// PBSGCStatus is garbage-collection job status for a datastore
// (GET /admin/gc, GET /admin/datastore/{store}/gc).
type PBSGCStatus struct {
	Store          string `json:"store" yaml:"store"`
	Schedule       string `json:"schedule,omitempty" yaml:"schedule,omitempty"`
	LastRunState   string `json:"last_run_state,omitempty" yaml:"last_run_state,omitempty"`
	LastRunEndtime int64  `json:"last_run_endtime,omitempty" yaml:"last_run_endtime,omitempty"`
	NextRun        int64  `json:"next_run,omitempty" yaml:"next_run,omitempty"`
	Duration       int64  `json:"duration,omitempty" yaml:"duration,omitempty"`
	UPID           string `json:"upid,omitempty" yaml:"upid,omitempty"`
	IndexFileCount int64  `json:"index_file_count" yaml:"index_file_count"`
	IndexDataBytes int64  `json:"index_data_bytes" yaml:"index_data_bytes"`
	DiskBytes      int64  `json:"disk_bytes" yaml:"disk_bytes"`
	DiskChunks     int64  `json:"disk_chunks" yaml:"disk_chunks"`
	RemovedBytes   int64  `json:"removed_bytes" yaml:"removed_bytes"`
	RemovedChunks  int64  `json:"removed_chunks" yaml:"removed_chunks"`
	PendingBytes   int64  `json:"pending_bytes" yaml:"pending_bytes"`
	PendingChunks  int64  `json:"pending_chunks" yaml:"pending_chunks"`
	RemovedBad     int64  `json:"removed_bad" yaml:"removed_bad"`
	StillBad       int64  `json:"still_bad" yaml:"still_bad"`
}

// PBSSubscription is subscription state (GET /nodes/{node}/subscription).
type PBSSubscription struct {
	Status      string `json:"status" yaml:"status"`
	Key         string `json:"key,omitempty" yaml:"key,omitempty"`
	Message     string `json:"message,omitempty" yaml:"message,omitempty"`
	ProductName string `json:"product_name,omitempty" yaml:"product_name,omitempty"`
	ServerID    string `json:"server_id,omitempty" yaml:"server_id,omitempty"`
	NextDueDate string `json:"next_due_date,omitempty" yaml:"next_due_date,omitempty"`
	CheckTime   int64  `json:"check_time,omitempty" yaml:"check_time,omitempty"`
	URL         string `json:"url,omitempty" yaml:"url,omitempty"`
}

// PBSCertificate is certificate information
// (GET /nodes/{node}/certificates/info).
type PBSCertificate struct {
	Filename      string   `json:"filename" yaml:"filename"`
	Subject       string   `json:"subject" yaml:"subject"`
	Issuer        string   `json:"issuer" yaml:"issuer"`
	Fingerprint   string   `json:"fingerprint,omitempty" yaml:"fingerprint,omitempty"`
	NotBefore     int64    `json:"notbefore,omitempty" yaml:"notbefore,omitempty"`
	NotAfter      int64    `json:"notafter,omitempty" yaml:"notafter,omitempty"`
	PublicKeyType string   `json:"public_key_type,omitempty" yaml:"public_key_type,omitempty"`
	PublicKeyBits int64    `json:"public_key_bits,omitempty" yaml:"public_key_bits,omitempty"`
	SAN           []string `json:"san,omitempty" yaml:"san,omitempty"`
}

// PBSSystemInspector exposes PBS host-level inspection.
type PBSSystemInspector interface {
	PBSVersionInfo(ctx context.Context) (*PBSVersionInfo, error)
	PBSNodeStatus(ctx context.Context) (*PBSNodeStatus, error)
	PBSSubscription(ctx context.Context) (*PBSSubscription, error)
	PBSCertificates(ctx context.Context) ([]PBSCertificate, error)
}

// PBSDatastoreInspector exposes datastore configuration and usage inspection.
type PBSDatastoreInspector interface {
	PBSDatastores(ctx context.Context) ([]PBSDatastore, error)
	PBSDatastore(ctx context.Context, name string) (*PBSDatastore, error)
	PBSDatastoreStatus(ctx context.Context, store string) (*PBSDatastoreStatus, error)
	PBSDatastoreUsages(ctx context.Context) ([]PBSDatastoreUsage, error)
}

// PBSSnapshotInspector exposes backup snapshot inspection.
type PBSSnapshotInspector interface {
	PBSSnapshots(ctx context.Context, store string, filter PBSSnapshotFilter) ([]PBSSnapshot, error)
}

// PBSTaskInspector exposes PBS task inspection.
type PBSTaskInspector interface {
	PBSTasks(ctx context.Context, filter PBSTaskFilter) ([]PBSTask, error)
	PBSTaskStatus(ctx context.Context, upid string) (*PBSTaskStatus, error)
	PBSTaskLog(ctx context.Context, upid string) ([]PBSTaskLogLine, error)
}

// PBSJobInspector exposes verify/prune/sync job configuration inspection.
type PBSJobInspector interface {
	PBSVerifyJobs(ctx context.Context) ([]PBSVerifyJob, error)
	PBSPruneJobs(ctx context.Context) ([]PBSPruneJob, error)
	PBSSyncJobs(ctx context.Context) ([]PBSSyncJob, error)
}

// PBSGCInspector exposes garbage-collection status inspection.
type PBSGCInspector interface {
	PBSGCStatuses(ctx context.Context) ([]PBSGCStatus, error)
	PBSGCStatus(ctx context.Context, store string) (*PBSGCStatus, error)
}
