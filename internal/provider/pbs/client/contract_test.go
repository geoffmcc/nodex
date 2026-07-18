package client

import (
	"encoding/json"
	"testing"
)

// Contract tests: decode realistic PBS API response payloads (field names
// taken from the official PBS API schema) into the typed response structs
// and assert the mapping. These guard against silent drift between the PBS
// API wire format and the client types. All values are fictional.

func TestContractVersion(t *testing.T) {
	payload := `{"data":{"version":"4.0.1","release":"1","repoid":"abcdef012345"}}`
	var resp VersionResponse
	if err := json.Unmarshal([]byte(payload), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Data.Version != "4.0.1" || resp.Data.Release != "1" || resp.Data.RepoID != "abcdef012345" {
		t.Errorf("unexpected mapping: %+v", resp.Data)
	}
}

func TestContractNodeStatus(t *testing.T) {
	payload := `{"data":{
		"cpu":0.031,"wait":0.002,"uptime":864000,
		"loadavg":[0.5,0.4,0.3],
		"kversion":"Linux 6.8.12-x-pbs #1 SMP",
		"memory":{"total":16777216000,"used":4294967296,"free":12482248704},
		"swap":{"total":8589934592,"used":0,"free":8589934592},
		"root":{"total":107374182400,"used":21474836480,"avail":85899345920},
		"cpuinfo":{"model":"Fictional CPU","cpus":8,"sockets":1},
		"boot-info":{"mode":"efi","secureboot":false},
		"info":{"fingerprint":"aa:bb:cc"}
	}}`
	var resp NodeStatusResponse
	if err := json.Unmarshal([]byte(payload), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	d := resp.Data
	if d.CPU != 0.031 || d.Uptime != 864000 {
		t.Errorf("cpu/uptime mapping wrong: %+v", d)
	}
	if d.Memory.Total != 16777216000 || d.Memory.Used != 4294967296 {
		t.Errorf("memory mapping wrong: %+v", d.Memory)
	}
	if d.Root.Avail != 85899345920 {
		t.Errorf("root mapping wrong: %+v", d.Root)
	}
	if d.CPUInfo.Model != "Fictional CPU" || d.CPUInfo.CPUs != 8 {
		t.Errorf("cpuinfo mapping wrong: %+v", d.CPUInfo)
	}
	if d.BootInfo.Mode != "efi" {
		t.Errorf("boot-info mapping wrong: %+v", d.BootInfo)
	}
	if len(d.LoadAvg) != 3 || d.LoadAvg[0] != 0.5 {
		t.Errorf("loadavg mapping wrong: %v", d.LoadAvg)
	}
}

func TestContractDatastoreConfig(t *testing.T) {
	payload := `{"data":[{
		"name":"backups","path":"/mnt/datastore/backups",
		"comment":"main datastore","gc-schedule":"daily",
		"prune-schedule":"daily","keep-last":3,"keep-daily":7,
		"keep-weekly":4,"keep-monthly":6,"verify-new":true,
		"maintenance-mode":"offline","notify":"gc=never"
	}]}`
	var resp DatastoreListResponse
	if err := json.Unmarshal([]byte(payload), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 datastore, got %d", len(resp.Data))
	}
	d := resp.Data[0]
	if d.Name != "backups" || d.Path != "/mnt/datastore/backups" {
		t.Errorf("name/path mapping wrong: %+v", d)
	}
	if d.GCSchedule != "daily" || d.PruneSchedule != "daily" {
		t.Errorf("schedule mapping wrong: %+v", d)
	}
	if d.KeepLast != 3 || d.KeepDaily != 7 || d.KeepWeekly != 4 || d.KeepMonthly != 6 {
		t.Errorf("keep mapping wrong: %+v", d)
	}
	if !d.VerifyNew || d.MaintenanceMode != "offline" {
		t.Errorf("verify-new/maintenance-mode mapping wrong: %+v", d)
	}
}

func TestContractSnapshots(t *testing.T) {
	payload := `{"data":[
		{
			"backup-type":"vm","backup-id":"100","backup-time":1752000000,
			"size":10737418240,"owner":"automation@pbs!nodex","protected":true,
			"comment":"nightly","fingerprint":"ab:cd",
			"files":[{"filename":"drive-scsi0.img.fidx","crypt-mode":"encrypt","size":10737418240}],
			"verification":{"state":"ok","upid":"UPID:pbs:0000AAAA:0000BBBB:00000001:65f00000:verificationjob:backups:automation@pbs!nodex:"}
		},
		{
			"backup-type":"host","backup-id":"pi-dns","backup-time":1752003600,
			"protected":false,
			"files":["catalog.pcat1.didx","index.json.blob"]
		}
	]}`
	var resp SnapshotListResponse
	if err := json.Unmarshal([]byte(payload), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(resp.Data))
	}
	vm := resp.Data[0]
	if vm.BackupType != "vm" || vm.BackupID != "100" || vm.BackupTime != 1752000000 {
		t.Errorf("vm snapshot mapping wrong: %+v", vm)
	}
	if !vm.Protected || vm.Size != 10737418240 {
		t.Errorf("protected/size mapping wrong: %+v", vm)
	}
	if len(vm.Files) != 1 || vm.Files[0].Filename != "drive-scsi0.img.fidx" {
		t.Errorf("object-form files mapping wrong: %+v", vm.Files)
	}
	if vm.Verification == nil || vm.Verification.State != "ok" {
		t.Errorf("verification mapping wrong: %+v", vm.Verification)
	}
	host := resp.Data[1]
	if len(host.Files) != 2 || host.Files[0].Filename != "catalog.pcat1.didx" {
		t.Errorf("string-form files mapping wrong: %+v", host.Files)
	}
	if host.Verification != nil {
		t.Errorf("expected nil verification, got %+v", host.Verification)
	}
}

func TestContractTasks(t *testing.T) {
	payload := `{"data":[{
		"upid":"UPID:pbs:00001234:00005678:00000001:65f00000:garbage_collection:backups:automation@pbs!nodex:",
		"node":"pbs","pid":4660,"pstart":22136,
		"starttime":1752000000,"endtime":1752000300,
		"worker_type":"garbage_collection","worker_id":"backups",
		"user":"automation@pbs!nodex","status":"OK"
	}]}`
	var resp TaskListResponse
	if err := json.Unmarshal([]byte(payload), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	tk := resp.Data[0]
	if tk.WorkerType != "garbage_collection" || tk.WorkerID != "backups" {
		t.Errorf("worker mapping wrong: %+v", tk)
	}
	if tk.StartTime != 1752000000 || tk.EndTime != 1752000300 || tk.Status != "OK" {
		t.Errorf("time/status mapping wrong: %+v", tk)
	}
}

func TestContractTaskStatus(t *testing.T) {
	// Note: the status endpoint uses "type"/"id", unlike the listing's
	// "worker_type"/"worker_id".
	payload := `{"data":{
		"upid":"UPID:pbs:00001234:00005678:00000001:65f00000:verificationjob:backups:automation@pbs!nodex:",
		"node":"pbs","pid":4660,"pstart":22136,"starttime":1752000000,
		"type":"verificationjob","id":"backups",
		"user":"automation@pbs!nodex","status":"stopped","exitstatus":"OK"
	}}`
	var resp TaskStatusResponse
	if err := json.Unmarshal([]byte(payload), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	d := resp.Data
	if d.Type != "verificationjob" || d.ID != "backups" {
		t.Errorf("type/id mapping wrong: %+v", d)
	}
	if d.Status != "stopped" || d.ExitStatus != "OK" {
		t.Errorf("status mapping wrong: %+v", d)
	}
}

func TestContractTaskLog(t *testing.T) {
	payload := `{"data":[{"n":1,"t":"starting garbage collection"},{"n":2,"t":"TASK OK"}],"total":2}`
	var resp TaskLogResponse
	if err := json.Unmarshal([]byte(payload), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 2 || resp.Data[0].N != 1 || resp.Data[1].T != "TASK OK" {
		t.Errorf("task log mapping wrong: %+v", resp.Data)
	}
}

func TestContractVerifyJobs(t *testing.T) {
	payload := `{"data":[{
		"id":"v-daily","store":"backups","ns":"prod","schedule":"daily",
		"comment":"verify all","ignore-verified":true,"outdated-after":30,"max-depth":2
	}]}`
	var resp VerifyJobListResponse
	if err := json.Unmarshal([]byte(payload), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	j := resp.Data[0]
	if j.ID != "v-daily" || j.Store != "backups" || j.NS != "prod" {
		t.Errorf("id/store/ns mapping wrong: %+v", j)
	}
	if !j.IgnoreVerified || j.OutdatedAfter != 30 || j.MaxDepth != 2 {
		t.Errorf("options mapping wrong: %+v", j)
	}
}

func TestContractPruneJobs(t *testing.T) {
	payload := `{"data":[{
		"id":"p-daily","store":"backups","schedule":"daily","disable":true,
		"keep-last":3,"keep-hourly":24,"keep-daily":7,"keep-weekly":4,
		"keep-monthly":6,"keep-yearly":1,"max-depth":1
	}]}`
	var resp PruneJobListResponse
	if err := json.Unmarshal([]byte(payload), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	j := resp.Data[0]
	if !j.Disable || j.KeepLast != 3 || j.KeepHourly != 24 || j.KeepYearly != 1 {
		t.Errorf("prune mapping wrong: %+v", j)
	}
}

func TestContractSyncJobs(t *testing.T) {
	payload := `{"data":[{
		"id":"s-offsite","store":"backups","ns":"prod",
		"remote":"offsite","remote-store":"replica","remote-ns":"main",
		"owner":"automation@pbs!nodex","schedule":"hourly",
		"remove-vanished":true,"sync-direction":"pull"
	}]}`
	var resp SyncJobListResponse
	if err := json.Unmarshal([]byte(payload), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	j := resp.Data[0]
	if j.Remote != "offsite" || j.RemoteStore != "replica" || j.RemoteNS != "main" {
		t.Errorf("remote mapping wrong: %+v", j)
	}
	if !j.RemoveVanished || j.SyncDirection != "pull" {
		t.Errorf("options mapping wrong: %+v", j)
	}
}

func TestContractGCStatus(t *testing.T) {
	payload := `{"data":{
		"store":"backups","schedule":"daily","last-run-state":"OK",
		"last-run-endtime":1752000300,"next-run":1752086400,"duration":300,
		"upid":"UPID:pbs:00001234:00005678:00000001:65f00000:garbage_collection:backups:automation@pbs!nodex:",
		"index-file-count":120,"index-data-bytes":107374182400,
		"disk-bytes":85899345920,"disk-chunks":40000,
		"removed-bytes":1073741824,"removed-chunks":500,
		"pending-bytes":536870912,"pending-chunks":250,
		"removed-bad":0,"still-bad":0
	}}`
	var resp GCStatusResponse
	if err := json.Unmarshal([]byte(payload), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	g := resp.Data
	if g.Store != "backups" || g.LastRunState != "OK" || g.LastRunEndtime != 1752000300 {
		t.Errorf("gc run mapping wrong: %+v", g)
	}
	if g.RemovedBytes != 1073741824 || g.PendingChunks != 250 || g.IndexFileCount != 120 {
		t.Errorf("gc stats mapping wrong: %+v", g)
	}
}

func TestContractDatastoreUsage(t *testing.T) {
	payload := `{"data":[{
		"store":"backups","total":214748364800,"used":107374182400,"avail":107374182400,
		"mount-status":"ok","estimated-full-date":1783536000,
		"history":[0.4,0.45,0.5],"history-delta":86400,"history-start":1751740800
	},{
		"store":"removable","mount-status":"notmounted","error":"not mounted"
	}]}`
	var resp DatastoreUsageResponse
	if err := json.Unmarshal([]byte(payload), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("expected 2 usage rows, got %d", len(resp.Data))
	}
	u := resp.Data[0]
	if u.Store != "backups" || u.Total != 214748364800 || u.MountStatus != "ok" {
		t.Errorf("usage mapping wrong: %+v", u)
	}
	if u.EstimatedFullDate != 1783536000 {
		t.Errorf("estimated-full-date mapping wrong: %+v", u)
	}
	broken := resp.Data[1]
	if broken.Error != "not mounted" || broken.MountStatus != "notmounted" {
		t.Errorf("error row mapping wrong: %+v", broken)
	}
}

func TestContractSubscription(t *testing.T) {
	payload := `{"data":{
		"status":"notfound","message":"There is no subscription key",
		"serverid":"0000000000000000","url":"https://www.proxmox.com"
	}}`
	var resp SubscriptionResponse
	if err := json.Unmarshal([]byte(payload), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Data.Status != "notfound" || resp.Data.ServerID != "0000000000000000" {
		t.Errorf("subscription mapping wrong: %+v", resp.Data)
	}
}

func TestContractCertificates(t *testing.T) {
	payload := `{"data":[{
		"filename":"proxy.pem","subject":"CN=pbs.example.invalid",
		"issuer":"CN=Fictional CA","fingerprint":"aa:bb:cc:dd",
		"notbefore":1720000000,"notafter":1783072000,
		"public-key-type":"id-ecPublicKey","public-key-bits":384,
		"san":["pbs.example.invalid","pbs"]
	}]}`
	var resp CertificateListResponse
	if err := json.Unmarshal([]byte(payload), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	c := resp.Data[0]
	if c.Filename != "proxy.pem" || c.Subject != "CN=pbs.example.invalid" {
		t.Errorf("cert mapping wrong: %+v", c)
	}
	if c.PublicKeyType != "id-ecPublicKey" || c.PublicKeyBits != 384 || c.NotAfter != 1783072000 {
		t.Errorf("cert key/date mapping wrong: %+v", c)
	}
	if len(c.SAN) != 2 || c.SAN[0] != "pbs.example.invalid" {
		t.Errorf("san mapping wrong: %v", c.SAN)
	}
}
