package proxmox

import (
	"testing"

	"github.com/geoffmcc/nodex/internal/provider/proxmox/client"
)

func TestMapTaskUsesExitStatusForTaskStatusResponse(t *testing.T) {
	task := mapTask(client.TaskListItem{
		UPID:       "UPID:pve-test:00002183:000434BF:6A56BE4B:qmstart:100:root@pam!token:",
		Type:       "qmstart",
		Status:     "stopped",
		ExitStatus: "OK",
	}, "pve-test")

	if task.State != "stopped" {
		t.Fatalf("State = %q, want stopped", task.State)
	}
	if task.Status != "OK" {
		t.Fatalf("Status = %q, want OK", task.Status)
	}
}

// The status endpoint reports the final outcome in exitstatus. A completed task
// carrying "WARNINGS: n" succeeded, and mapTask must surface that exit status
// so callers classify it consistently with the poller.
func TestMapTaskPreservesWarningExitStatus(t *testing.T) {
	task := mapTask(client.TaskListItem{
		UPID:       "UPID:pve-test:00002183:000434BF:6A56BE4B:vzstart:100:root@pam!token:",
		Type:       "vzstart",
		Status:     "stopped",
		ExitStatus: "WARNINGS: 1",
	}, "pve-test")

	if task.State != "stopped" {
		t.Fatalf("State = %q, want stopped", task.State)
	}
	if task.Status != "WARNINGS: 1" {
		t.Fatalf("Status = %q, want WARNINGS: 1", task.Status)
	}
}

func TestMapTaskPreservesTaskListRow(t *testing.T) {
	task := mapTask(client.TaskListItem{
		UPID:   "UPID:pve-test:00000001",
		Type:   "vzdump",
		State:  "stopped",
		Status: "OK",
	}, "pve-test")

	if task.State != "stopped" {
		t.Fatalf("State = %q, want stopped", task.State)
	}
	if task.Status != "OK" {
		t.Fatalf("Status = %q, want OK", task.Status)
	}
}

func TestMapTaskInfersStateWhenProxmoxOmitsIt(t *testing.T) {
	task := mapTask(client.TaskListItem{
		UPID:      "UPID:pve-test:00000002",
		Type:      "qmstart",
		StartTime: 100,
		EndTime:   101,
		Status:    "OK",
	}, "pve-test")

	if task.State != "stopped" {
		t.Fatalf("State = %q, want stopped", task.State)
	}
}
