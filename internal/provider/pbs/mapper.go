package pbs

import (
	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/provider/pbs/client"
)

// MapVersion converts client version data to the domain model.
func MapVersion(v *client.VersionData) *domain.PBSVersionInfo {
	return &domain.PBSVersionInfo{
		Version: v.Version,
		Release: v.Release,
		RepoID:  v.RepoID,
	}
}

// MapNodeStatus converts client node status to the domain model.
func MapNodeStatus(s *client.NodeStatusData) *domain.PBSNodeStatus {
	return &domain.PBSNodeStatus{
		CPU:           s.CPU,
		Wait:          s.Wait,
		Uptime:        s.Uptime,
		LoadAvg:       s.LoadAvg,
		KernelVersion: s.KVersion,
		CPUModel:      s.CPUInfo.Model,
		CPUs:          s.CPUInfo.CPUs,
		MemoryTotal:   s.Memory.Total,
		MemoryUsed:    s.Memory.Used,
		MemoryFree:    s.Memory.Free,
		SwapTotal:     s.Swap.Total,
		SwapUsed:      s.Swap.Used,
		RootTotal:     s.Root.Total,
		RootUsed:      s.Root.Used,
		RootAvail:     s.Root.Avail,
		BootMode:      s.BootInfo.Mode,
	}
}

// MapDatastore converts one datastore config to the domain model.
func MapDatastore(d client.DatastoreConfig) domain.PBSDatastore {
	return domain.PBSDatastore{
		Name:            d.Name,
		Path:            d.Path,
		Comment:         d.Comment,
		GCSchedule:      d.GCSchedule,
		PruneSchedule:   d.PruneSchedule,
		KeepLast:        d.KeepLast,
		KeepHourly:      d.KeepHourly,
		KeepDaily:       d.KeepDaily,
		KeepWeekly:      d.KeepWeekly,
		KeepMonthly:     d.KeepMonthly,
		KeepYearly:      d.KeepYearly,
		VerifyNew:       d.VerifyNew,
		MaintenanceMode: d.MaintenanceMode,
		Backend:         d.Backend,
	}
}

// MapDatastores converts datastore configs to domain models.
func MapDatastores(items []client.DatastoreConfig) []domain.PBSDatastore {
	out := make([]domain.PBSDatastore, 0, len(items))
	for _, d := range items {
		out = append(out, MapDatastore(d))
	}
	return out
}

// MapDatastoreStatus converts datastore status to the domain model. The
// store name comes from the request; the API payload does not repeat it.
func MapDatastoreStatus(store string, s *client.DatastoreStatusData) *domain.PBSDatastoreStatus {
	return &domain.PBSDatastoreStatus{
		Store:       store,
		Total:       s.Total,
		Used:        s.Used,
		Avail:       s.Avail,
		BackendType: s.BackendType,
	}
}

// MapDatastoreUsages converts datastore usage summaries to domain models.
func MapDatastoreUsages(items []client.DatastoreUsageItem) []domain.PBSDatastoreUsage {
	out := make([]domain.PBSDatastoreUsage, 0, len(items))
	for _, u := range items {
		out = append(out, domain.PBSDatastoreUsage{
			Store:             u.Store,
			Total:             u.Total,
			Used:              u.Used,
			Avail:             u.Avail,
			MountStatus:       u.MountStatus,
			EstimatedFullDate: u.EstimatedFullDate,
			Error:             u.Error,
		})
	}
	return out
}

// MapSnapshots converts snapshot items to domain models. Store and namespace
// come from the request; the API payload does not repeat them.
func MapSnapshots(store, namespace string, items []client.SnapshotItem) []domain.PBSSnapshot {
	out := make([]domain.PBSSnapshot, 0, len(items))
	for _, s := range items {
		snap := domain.PBSSnapshot{
			Store:       store,
			Namespace:   namespace,
			BackupType:  s.BackupType,
			BackupID:    s.BackupID,
			BackupTime:  s.BackupTime,
			Size:        s.Size,
			Owner:       s.Owner,
			Protected:   s.Protected,
			Comment:     s.Comment,
			Fingerprint: s.Fingerprint,
		}
		for _, f := range s.Files {
			snap.Files = append(snap.Files, f.Filename)
		}
		if s.Verification != nil {
			snap.Verification = &domain.PBSVerificationState{
				State: s.Verification.State,
				UPID:  s.Verification.UPID,
			}
		}
		out = append(out, snap)
	}
	return out
}

// MapTasks converts task items to domain models.
func MapTasks(items []client.TaskItem) []domain.PBSTask {
	out := make([]domain.PBSTask, 0, len(items))
	for _, t := range items {
		out = append(out, domain.PBSTask{
			UPID:       t.UPID,
			Node:       t.Node,
			WorkerType: t.WorkerType,
			WorkerID:   t.WorkerID,
			User:       t.User,
			StartTime:  t.StartTime,
			EndTime:    t.EndTime,
			Status:     t.Status,
		})
	}
	return out
}

// MapTaskStatus converts detailed task state to the domain model.
func MapTaskStatus(s *client.TaskStatusData) *domain.PBSTaskStatus {
	return &domain.PBSTaskStatus{
		UPID:       s.UPID,
		Node:       s.Node,
		PID:        s.PID,
		WorkerType: s.Type,
		WorkerID:   s.ID,
		User:       s.User,
		StartTime:  s.StartTime,
		EndTime:    s.EndTime,
		Status:     s.Status,
		ExitStatus: s.ExitStatus,
	}
}

// MapTaskLog converts task log lines to domain models.
func MapTaskLog(lines []client.TaskLogLine) []domain.PBSTaskLogLine {
	out := make([]domain.PBSTaskLogLine, 0, len(lines))
	for _, l := range lines {
		out = append(out, domain.PBSTaskLogLine{LineNumber: l.N, Text: l.T})
	}
	return out
}

// MapVerifyJobs converts verify job configs to domain models.
func MapVerifyJobs(items []client.VerifyJobConfig) []domain.PBSVerifyJob {
	out := make([]domain.PBSVerifyJob, 0, len(items))
	for _, j := range items {
		out = append(out, domain.PBSVerifyJob{
			ID:             j.ID,
			Store:          j.Store,
			Namespace:      j.NS,
			Schedule:       j.Schedule,
			Comment:        j.Comment,
			IgnoreVerified: j.IgnoreVerified,
			OutdatedAfter:  j.OutdatedAfter,
			MaxDepth:       j.MaxDepth,
		})
	}
	return out
}

// MapPruneJobs converts prune job configs to domain models.
func MapPruneJobs(items []client.PruneJobConfig) []domain.PBSPruneJob {
	out := make([]domain.PBSPruneJob, 0, len(items))
	for _, j := range items {
		out = append(out, domain.PBSPruneJob{
			ID:          j.ID,
			Store:       j.Store,
			Namespace:   j.NS,
			Schedule:    j.Schedule,
			Comment:     j.Comment,
			Disable:     j.Disable,
			KeepLast:    j.KeepLast,
			KeepHourly:  j.KeepHourly,
			KeepDaily:   j.KeepDaily,
			KeepWeekly:  j.KeepWeekly,
			KeepMonthly: j.KeepMonthly,
			KeepYearly:  j.KeepYearly,
			MaxDepth:    j.MaxDepth,
		})
	}
	return out
}

// MapSyncJobs converts sync job configs to domain models.
func MapSyncJobs(items []client.SyncJobConfig) []domain.PBSSyncJob {
	out := make([]domain.PBSSyncJob, 0, len(items))
	for _, j := range items {
		out = append(out, domain.PBSSyncJob{
			ID:              j.ID,
			Store:           j.Store,
			Namespace:       j.NS,
			Remote:          j.Remote,
			RemoteStore:     j.RemoteStore,
			RemoteNamespace: j.RemoteNS,
			Owner:           j.Owner,
			Schedule:        j.Schedule,
			Comment:         j.Comment,
			RemoveVanished:  j.RemoveVanished,
			SyncDirection:   j.SyncDirection,
		})
	}
	return out
}

// MapGCStatus converts one GC status to the domain model.
func MapGCStatus(g client.GCStatusData) domain.PBSGCStatus {
	return domain.PBSGCStatus{
		Store:          g.Store,
		Schedule:       g.Schedule,
		LastRunState:   g.LastRunState,
		LastRunEndtime: g.LastRunEndtime,
		NextRun:        g.NextRun,
		Duration:       g.Duration,
		UPID:           g.UPID,
		IndexFileCount: g.IndexFileCount,
		IndexDataBytes: g.IndexDataBytes,
		DiskBytes:      g.DiskBytes,
		DiskChunks:     g.DiskChunks,
		RemovedBytes:   g.RemovedBytes,
		RemovedChunks:  g.RemovedChunks,
		PendingBytes:   g.PendingBytes,
		PendingChunks:  g.PendingChunks,
		RemovedBad:     g.RemovedBad,
		StillBad:       g.StillBad,
	}
}

// MapGCStatuses converts GC statuses to domain models.
func MapGCStatuses(items []client.GCStatusData) []domain.PBSGCStatus {
	out := make([]domain.PBSGCStatus, 0, len(items))
	for _, g := range items {
		out = append(out, MapGCStatus(g))
	}
	return out
}

// MapSubscription converts subscription data to the domain model.
func MapSubscription(s *client.SubscriptionData) *domain.PBSSubscription {
	return &domain.PBSSubscription{
		Status:      s.Status,
		Key:         s.Key,
		Message:     s.Message,
		ProductName: s.ProductName,
		ServerID:    s.ServerID,
		NextDueDate: s.NextDueDate,
		CheckTime:   s.CheckTime,
		URL:         s.URL,
	}
}

// MapCertificates converts certificate info to domain models.
func MapCertificates(items []client.CertificateInfo) []domain.PBSCertificate {
	out := make([]domain.PBSCertificate, 0, len(items))
	for _, c := range items {
		out = append(out, domain.PBSCertificate{
			Filename:      c.Filename,
			Subject:       c.Subject,
			Issuer:        c.Issuer,
			Fingerprint:   c.Fingerprint,
			NotBefore:     c.NotBefore,
			NotAfter:      c.NotAfter,
			PublicKeyType: c.PublicKeyType,
			PublicKeyBits: c.PublicKeyBits,
			SAN:           c.SAN,
		})
	}
	return out
}
