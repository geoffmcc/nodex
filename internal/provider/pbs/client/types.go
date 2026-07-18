package client

import "encoding/json"

// Typed request/response objects for the Proxmox Backup Server HTTP API.
// JSON field names follow the PBS API schema exactly (kebab-case where PBS
// uses kebab-case). All responses arrive wrapped in {"data": ...}.

// VersionResponse wraps GET /version.
type VersionResponse struct {
	Data VersionData `json:"data"`
}

// VersionData is the PBS version payload.
type VersionData struct {
	Version string `json:"version"`
	Release string `json:"release"`
	RepoID  string `json:"repoid"`
}

// NodeStatusResponse wraps GET /nodes/{node}/status.
type NodeStatusResponse struct {
	Data NodeStatusData `json:"data"`
}

// NodeStatusData is the PBS host status payload.
type NodeStatusData struct {
	CPU      float64        `json:"cpu"`
	Wait     float64        `json:"wait"`
	Uptime   int64          `json:"uptime"`
	LoadAvg  []float64      `json:"loadavg"`
	KVersion string         `json:"kversion"`
	Memory   MemoryStatus   `json:"memory"`
	Swap     MemoryStatus   `json:"swap"`
	Root     RootStatus     `json:"root"`
	CPUInfo  CPUInfoStatus  `json:"cpuinfo"`
	BootInfo BootInfoStatus `json:"boot-info"`
}

// MemoryStatus holds total/used/free byte counts.
type MemoryStatus struct {
	Total int64 `json:"total"`
	Used  int64 `json:"used"`
	Free  int64 `json:"free"`
}

// RootStatus holds root filesystem usage.
type RootStatus struct {
	Total int64 `json:"total"`
	Used  int64 `json:"used"`
	Avail int64 `json:"avail"`
}

// CPUInfoStatus describes the host CPU.
type CPUInfoStatus struct {
	Model   string `json:"model"`
	CPUs    int64  `json:"cpus"`
	Sockets int64  `json:"sockets"`
}

// BootInfoStatus describes the boot mode.
type BootInfoStatus struct {
	Mode       string `json:"mode"`
	SecureBoot bool   `json:"secureboot"`
}

// DatastoreListResponse wraps GET /config/datastore.
type DatastoreListResponse struct {
	Data []DatastoreConfig `json:"data"`
}

// DatastoreResponse wraps GET /config/datastore/{name}.
type DatastoreResponse struct {
	Data DatastoreConfig `json:"data"`
}

// DatastoreConfig is a datastore configuration entry.
type DatastoreConfig struct {
	Name            string `json:"name"`
	Path            string `json:"path"`
	Comment         string `json:"comment,omitempty"`
	GCSchedule      string `json:"gc-schedule,omitempty"`
	PruneSchedule   string `json:"prune-schedule,omitempty"`
	KeepLast        int64  `json:"keep-last,omitempty"`
	KeepHourly      int64  `json:"keep-hourly,omitempty"`
	KeepDaily       int64  `json:"keep-daily,omitempty"`
	KeepWeekly      int64  `json:"keep-weekly,omitempty"`
	KeepMonthly     int64  `json:"keep-monthly,omitempty"`
	KeepYearly      int64  `json:"keep-yearly,omitempty"`
	VerifyNew       bool   `json:"verify-new,omitempty"`
	MaintenanceMode string `json:"maintenance-mode,omitempty"`
	Backend         string `json:"backend,omitempty"`
}

// DatastoreStatusResponse wraps GET /admin/datastore/{store}/status.
type DatastoreStatusResponse struct {
	Data DatastoreStatusData `json:"data"`
}

// DatastoreStatusData is a datastore's usage payload.
type DatastoreStatusData struct {
	Total       int64  `json:"total"`
	Used        int64  `json:"used"`
	Avail       int64  `json:"avail"`
	BackendType string `json:"backend-type,omitempty"`
}

// DatastoreUsageResponse wraps GET /status/datastore-usage.
type DatastoreUsageResponse struct {
	Data []DatastoreUsageItem `json:"data"`
}

// DatastoreUsageItem is one datastore usage summary.
type DatastoreUsageItem struct {
	Store             string `json:"store"`
	Total             int64  `json:"total,omitempty"`
	Used              int64  `json:"used,omitempty"`
	Avail             int64  `json:"avail,omitempty"`
	MountStatus       string `json:"mount-status,omitempty"`
	EstimatedFullDate int64  `json:"estimated-full-date,omitempty"`
	Error             string `json:"error,omitempty"`
}

// SnapshotListResponse wraps GET /admin/datastore/{store}/snapshots.
type SnapshotListResponse struct {
	Data []SnapshotItem `json:"data"`
}

// SnapshotItem is one backup snapshot.
type SnapshotItem struct {
	BackupType   string            `json:"backup-type"`
	BackupID     string            `json:"backup-id"`
	BackupTime   int64             `json:"backup-time"`
	Size         int64             `json:"size,omitempty"`
	Owner        string            `json:"owner,omitempty"`
	Protected    bool              `json:"protected"`
	Comment      string            `json:"comment,omitempty"`
	Fingerprint  string            `json:"fingerprint,omitempty"`
	Files        []SnapshotFile    `json:"files,omitempty"`
	Verification *VerificationItem `json:"verification,omitempty"`
}

// SnapshotFile is one file inside a snapshot. PBS returns either plain
// strings or objects here depending on server version; UnmarshalJSON accepts
// both shapes.
type SnapshotFile struct {
	Filename  string `json:"filename"`
	CryptMode string `json:"crypt-mode,omitempty"`
	Size      int64  `json:"size,omitempty"`
}

// UnmarshalJSON accepts both the string form ("catalog.pcat1.didx") and the
// object form ({"filename": ..., "crypt-mode": ..., "size": ...}).
func (f *SnapshotFile) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		var name string
		if err := json.Unmarshal(data, &name); err != nil {
			return err
		}
		*f = SnapshotFile{Filename: name}
		return nil
	}
	type rawFile SnapshotFile
	var raw rawFile
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*f = SnapshotFile(raw)
	return nil
}

// VerificationItem is a snapshot verification result.
type VerificationItem struct {
	State string `json:"state"`
	UPID  string `json:"upid"`
}

// TaskListResponse wraps GET /nodes/{node}/tasks.
type TaskListResponse struct {
	Data []TaskItem `json:"data"`
}

// TaskItem is one task listing entry.
type TaskItem struct {
	UPID       string `json:"upid"`
	Node       string `json:"node"`
	PID        int64  `json:"pid"`
	PStart     int64  `json:"pstart"`
	StartTime  int64  `json:"starttime"`
	EndTime    int64  `json:"endtime,omitempty"`
	WorkerType string `json:"worker_type"`
	WorkerID   string `json:"worker_id,omitempty"`
	User       string `json:"user"`
	Status     string `json:"status,omitempty"`
}

// TaskStatusResponse wraps GET /nodes/{node}/tasks/{upid}/status.
type TaskStatusResponse struct {
	Data TaskStatusData `json:"data"`
}

// TaskStatusData is detailed task state.
type TaskStatusData struct {
	UPID       string `json:"upid"`
	Node       string `json:"node"`
	PID        int64  `json:"pid"`
	PStart     int64  `json:"pstart"`
	StartTime  int64  `json:"starttime"`
	EndTime    int64  `json:"endtime,omitempty"`
	Type       string `json:"type"`
	ID         string `json:"id,omitempty"`
	User       string `json:"user"`
	Status     string `json:"status"`
	ExitStatus string `json:"exitstatus,omitempty"`
}

// TaskLogResponse wraps GET /nodes/{node}/tasks/{upid}/log.
type TaskLogResponse struct {
	Data []TaskLogLine `json:"data"`
}

// TaskLogLine is one task log line.
type TaskLogLine struct {
	N int64  `json:"n"`
	T string `json:"t"`
}

// VerifyJobListResponse wraps GET /config/verify.
type VerifyJobListResponse struct {
	Data []VerifyJobConfig `json:"data"`
}

// VerifyJobConfig is one verification job.
type VerifyJobConfig struct {
	ID             string `json:"id"`
	Store          string `json:"store"`
	NS             string `json:"ns,omitempty"`
	Schedule       string `json:"schedule,omitempty"`
	Comment        string `json:"comment,omitempty"`
	IgnoreVerified bool   `json:"ignore-verified,omitempty"`
	OutdatedAfter  int64  `json:"outdated-after,omitempty"`
	MaxDepth       int64  `json:"max-depth,omitempty"`
}

// PruneJobListResponse wraps GET /config/prune.
type PruneJobListResponse struct {
	Data []PruneJobConfig `json:"data"`
}

// PruneJobConfig is one prune job.
type PruneJobConfig struct {
	ID          string `json:"id"`
	Store       string `json:"store"`
	NS          string `json:"ns,omitempty"`
	Schedule    string `json:"schedule"`
	Comment     string `json:"comment,omitempty"`
	Disable     bool   `json:"disable,omitempty"`
	KeepLast    int64  `json:"keep-last,omitempty"`
	KeepHourly  int64  `json:"keep-hourly,omitempty"`
	KeepDaily   int64  `json:"keep-daily,omitempty"`
	KeepWeekly  int64  `json:"keep-weekly,omitempty"`
	KeepMonthly int64  `json:"keep-monthly,omitempty"`
	KeepYearly  int64  `json:"keep-yearly,omitempty"`
	MaxDepth    int64  `json:"max-depth,omitempty"`
}

// SyncJobListResponse wraps GET /config/sync.
type SyncJobListResponse struct {
	Data []SyncJobConfig `json:"data"`
}

// SyncJobConfig is one sync job.
type SyncJobConfig struct {
	ID             string `json:"id"`
	Store          string `json:"store"`
	NS             string `json:"ns,omitempty"`
	Remote         string `json:"remote,omitempty"`
	RemoteStore    string `json:"remote-store"`
	RemoteNS       string `json:"remote-ns,omitempty"`
	Owner          string `json:"owner,omitempty"`
	Schedule       string `json:"schedule,omitempty"`
	Comment        string `json:"comment,omitempty"`
	RemoveVanished bool   `json:"remove-vanished,omitempty"`
	SyncDirection  string `json:"sync-direction,omitempty"`
}

// GCListResponse wraps GET /admin/gc.
type GCListResponse struct {
	Data []GCStatusData `json:"data"`
}

// GCStatusResponse wraps GET /admin/datastore/{store}/gc.
type GCStatusResponse struct {
	Data GCStatusData `json:"data"`
}

// GCStatusData is garbage-collection status for one datastore.
type GCStatusData struct {
	Store          string `json:"store"`
	Schedule       string `json:"schedule,omitempty"`
	LastRunState   string `json:"last-run-state,omitempty"`
	LastRunEndtime int64  `json:"last-run-endtime,omitempty"`
	NextRun        int64  `json:"next-run,omitempty"`
	Duration       int64  `json:"duration,omitempty"`
	UPID           string `json:"upid,omitempty"`
	IndexFileCount int64  `json:"index-file-count"`
	IndexDataBytes int64  `json:"index-data-bytes"`
	DiskBytes      int64  `json:"disk-bytes"`
	DiskChunks     int64  `json:"disk-chunks"`
	RemovedBytes   int64  `json:"removed-bytes"`
	RemovedChunks  int64  `json:"removed-chunks"`
	PendingBytes   int64  `json:"pending-bytes"`
	PendingChunks  int64  `json:"pending-chunks"`
	RemovedBad     int64  `json:"removed-bad"`
	StillBad       int64  `json:"still-bad"`
}

// SubscriptionResponse wraps GET /nodes/{node}/subscription.
type SubscriptionResponse struct {
	Data SubscriptionData `json:"data"`
}

// SubscriptionData is subscription state.
type SubscriptionData struct {
	Status      string `json:"status"`
	Key         string `json:"key,omitempty"`
	Message     string `json:"message,omitempty"`
	ProductName string `json:"productname,omitempty"`
	ServerID    string `json:"serverid,omitempty"`
	NextDueDate string `json:"nextduedate,omitempty"`
	RegDate     string `json:"regdate,omitempty"`
	CheckTime   int64  `json:"checktime,omitempty"`
	URL         string `json:"url,omitempty"`
}

// CertificateListResponse wraps GET /nodes/{node}/certificates/info.
type CertificateListResponse struct {
	Data []CertificateInfo `json:"data"`
}

// CertificateInfo is one certificate description.
type CertificateInfo struct {
	Filename      string   `json:"filename"`
	Subject       string   `json:"subject"`
	Issuer        string   `json:"issuer"`
	Fingerprint   string   `json:"fingerprint,omitempty"`
	NotBefore     int64    `json:"notbefore,omitempty"`
	NotAfter      int64    `json:"notafter,omitempty"`
	PublicKeyType string   `json:"public-key-type"`
	PublicKeyBits int64    `json:"public-key-bits,omitempty"`
	SAN           []string `json:"san,omitempty"`
}
