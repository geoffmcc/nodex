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
