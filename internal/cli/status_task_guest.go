package cli

import (
	"strconv"
	"strings"

	"github.com/geoffmcc/nodex/internal/domain"
)

// statusTaskGuestLabel associates a Proxmox worker task with the guest it
// targets. The standard UPID's worker ID is a VMID for guest-scoped tasks.
// Inventory lookup supplies a name when available; host tasks remain unlabeled.
func statusTaskGuestLabel(task domain.Task, snapshot *statusOverview) string {
	upidNode, worker, vmid, ok := parseStatusTaskUPID(task.UPID)
	if !ok || (task.Node != "" && task.Node != upidNode) {
		return "-"
	}
	kind := ""
	switch {
	case strings.HasPrefix(worker, "qm"):
		kind = "VM"
	case strings.HasPrefix(worker, "vz"):
		kind = "CT"
	}
	if snapshot != nil {
		for _, guest := range snapshot.VMDetails {
			if statusGuestVMID(guest.ID) == vmid && (guest.Node == "" || guest.Node == upidNode) {
				return statusGuestLabel("VM", vmid, guest.Name)
			}
		}
		for _, guest := range snapshot.ContainerDetails {
			if statusGuestVMID(guest.ID) == vmid && (guest.Node == "" || guest.Node == upidNode) {
				return statusGuestLabel("CT", vmid, guest.Name)
			}
		}
	}
	if kind != "" {
		return statusGuestLabel(kind, vmid, "")
	}
	return "-"
}

func parseStatusTaskUPID(upid string) (node, worker string, vmid int, ok bool) {
	parts := strings.Split(upid, ":")
	// UPID:<node>:<pid>:<pstart>:<starttime>:<worker>:<worker-id>:<user>:
	if len(parts) < 8 || parts[0] != "UPID" || parts[1] == "" || parts[5] == "" {
		return "", "", 0, false
	}
	vmid, err := strconv.Atoi(parts[6])
	if err != nil || vmid <= 0 {
		return "", "", 0, false
	}
	return parts[1], parts[5], vmid, true
}

func statusGuestVMID(id string) int {
	if slash := strings.LastIndexByte(id, '/'); slash >= 0 {
		id = id[slash+1:]
	}
	vmid, _ := strconv.Atoi(id)
	return vmid
}

func statusGuestLabel(kind string, vmid int, name string) string {
	label := kind + " " + strconv.Itoa(vmid)
	if name != "" {
		label += " " + name
	}
	return label
}
