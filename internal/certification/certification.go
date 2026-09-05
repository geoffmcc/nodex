package certification

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/task"
)

const RequiredProfile = "nodex-test-admin"

type Request struct {
	Profile, Node, Name, Storage string
	VMID                         int
	ConfirmTarget                string
	OptIn                        bool
}

type Result struct {
	Profile string `json:"profile"`
	Target  string `json:"target"`
	State   string `json:"state"`
	Cleanup string `json:"cleanup"`
	Ledger  string `json:"ledger"`
	Error   string `json:"error,omitempty"`
}

type taskClient struct{ provider domain.TaskInspector }

func (c taskClient) GetTask(ctx context.Context, node, upid string) (*task.TaskStatus, error) {
	t, err := c.provider.Task(ctx, node, upid)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, fmt.Errorf("provider returned no task status")
	}
	state := task.StateRunning
	if t.State == "stopped" {
		state = task.StateStopped
	}
	return &task.TaskStatus{UPID: t.UPID, State: state, Status: t.Status}, nil
}

func validateRequest(r Request) error {
	if !r.OptIn {
		return fmt.Errorf("certification is opt-in; pass --yes")
	}
	if r.Profile != RequiredProfile {
		return fmt.Errorf("certification requires explicit profile %q", RequiredProfile)
	}
	if r.Node == "" || r.Storage == "" || r.VMID <= 0 {
		return fmt.Errorf("node, positive vmid, and storage are required")
	}
	for _, item := range []struct{ field, value string }{{"node", r.Node}, {"storage", r.Storage}, {"name", r.Name}} {
		field, value := item.field, item.value
		lower := strings.ToLower(value)
		for _, marker := range []string{"prod", "production", "live", "primary"} {
			if strings.Contains(lower, marker) {
				return fmt.Errorf("production-looking %s %q refused", field, value)
			}
		}
	}
	if !strings.HasPrefix(r.Name, NamePrefix) || len(r.Name) <= len(NamePrefix) {
		return fmt.Errorf("name must use the %q prefix", NamePrefix)
	}
	if r.ConfirmTarget != r.Name {
		return fmt.Errorf("--confirm-target must exactly equal %q", r.Name)
	}
	return nil
}

// ValidateRequest exposes the fail-closed input boundary for CLI and callers.
func ValidateRequest(r Request) error { return validateRequest(r) }

func waitTask(ctx context.Context, p domain.Provider, node, upid string) error {
	ti, ok := p.(domain.TaskInspector)
	if !ok {
		return fmt.Errorf("provider cannot verify certification tasks")
	}
	r := task.NewPoller(taskClient{ti}, task.WithMaxWait(10*time.Minute)).Wait(ctx, node, upid)
	if r.Error != nil {
		return r.Error
	}
	if !r.OK {
		return fmt.Errorf("task failed with status %q", r.Status)
	}
	return nil
}

func waitForVM(ctx context.Context, inspector domain.VMInspector, node string, vmid int, name string) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	targetID := fmt.Sprintf("%s/%d", node, vmid)
	for {
		vms, err := inspector.VMs(ctx)
		if err != nil {
			return err
		}
		for _, vm := range vms {
			if vm.ID == targetID && vm.Name == name {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("created certification VM was not visible after task completion")
		case <-ticker.C:
		}
	}
}

func Run(ctx context.Context, p domain.Provider, req Request, ledgerPath string, now time.Time) (Result, error) {
	if err := validateRequest(req); err != nil {
		return Result{}, err
	}
	creator, ok := p.(domain.VMCreateProvider)
	if !ok {
		return Result{}, fmt.Errorf("provider does not support VM creation")
	}
	deleter, ok := p.(domain.DeleteProvider)
	if !ok {
		return Result{}, fmt.Errorf("provider does not support VM cleanup")
	}
	inspector, ok := p.(domain.VMInspector)
	if !ok {
		return Result{}, fmt.Errorf("provider cannot verify VM state")
	}
	vms, err := inspector.VMs(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("preflight VMs: %w", err)
	}
	targetID := fmt.Sprintf("%s/%d", req.Node, req.VMID)
	for _, vm := range vms {
		if vm.ID == targetID || vm.Name == req.Name {
			return Result{}, fmt.Errorf("certification target already exists; refusing to overwrite")
		}
	}
	l, err := Load(ledgerPath)
	if err != nil {
		return Result{}, err
	}
	e := NewEntry(req.Profile, req.Node, req.VMID, req.Name, req.Storage, now)
	for _, prior := range l.Entries {
		if prior.Name == req.Name || prior.VMID == req.VMID && prior.Node == req.Node {
			return Result{}, fmt.Errorf("certification target is already present in the cleanup ledger")
		}
	}
	l.Entries = append(l.Entries, e)
	if err := Save(ledgerPath, l); err != nil {
		return Result{}, fmt.Errorf("reserve cleanup ledger: %w", err)
	}
	entryIndex := -1
	for i := range l.Entries {
		if l.Entries[i].ID == e.ID {
			entryIndex = i
			break
		}
	}
	if entryIndex < 0 {
		return Result{}, fmt.Errorf("reserved certification ledger entry disappeared")
	}
	result := Result{Profile: req.Profile, Target: req.Name, State: "reserved", Cleanup: "required", Ledger: ledgerPath}
	upid, err := creator.VMCreate(ctx, req.Node, req.VMID, req.Name, "", req.Storage)
	if err != nil {
		return result, fmt.Errorf("create certification VM: %w", err)
	}
	if err := waitTask(ctx, p, req.Node, upid); err != nil {
		return result, fmt.Errorf("create certification VM: %w", err)
	}
	l.Entries[entryIndex].State, l.Entries[entryIndex].UpdatedAt = "created", time.Now().Unix()
	_ = Save(ledgerPath, l)
	if err := waitForVM(ctx, inspector, req.Node, req.VMID, req.Name); err != nil {
		return result, fmt.Errorf("verify certification VM: %w", err)
	}
	deleteUPID, err := deleter.VMDelete(ctx, req.Node, req.VMID)
	if err != nil {
		return result, fmt.Errorf("cleanup certification VM: %w", err)
	}
	if err := waitTask(ctx, p, req.Node, deleteUPID); err != nil {
		return result, fmt.Errorf("cleanup certification VM: %w", err)
	}
	vms, err = inspector.VMs(ctx)
	if err != nil {
		return result, fmt.Errorf("verify certification cleanup: %w", err)
	}
	for _, vm := range vms {
		if vm.ID == targetID {
			return result, fmt.Errorf("certification cleanup could not verify target absence")
		}
	}
	l.Entries[entryIndex].State, l.Entries[entryIndex].Cleanup, l.Entries[entryIndex].UpdatedAt = "succeeded", "complete", time.Now().Unix()
	if err := Save(ledgerPath, l); err != nil {
		return result, err
	}
	result.State, result.Cleanup = "succeeded", "complete"
	return result, nil
}

func Cleanup(ctx context.Context, p domain.Provider, ledgerPath, confirm string) ([]Result, error) {
	if confirm == "" {
		return nil, fmt.Errorf("cleanup requires exact --confirm-target <ledger-entry-id>")
	}
	l, err := Load(ledgerPath)
	if err != nil {
		return nil, err
	}
	d, ok := p.(domain.DeleteProvider)
	if !ok {
		return nil, fmt.Errorf("provider does not support VM cleanup")
	}
	inspector, ok := p.(domain.VMInspector)
	if !ok {
		return nil, fmt.Errorf("provider cannot verify VM state")
	}
	var out []Result
	for i := range l.Entries {
		e := &l.Entries[i]
		if e.Cleanup == "complete" || e.ID != confirm {
			continue
		}
		if e.Profile != RequiredProfile || !strings.HasPrefix(e.Name, NamePrefix) {
			return nil, fmt.Errorf("ledger entry is outside certification safety boundary")
		}
		vms, err := inspector.VMs(ctx)
		if err != nil {
			return nil, err
		}
		targetID := fmt.Sprintf("%s/%d", e.Node, e.VMID)
		present := false
		for _, vm := range vms {
			if vm.ID == targetID {
				if vm.Name != e.Name {
					return nil, fmt.Errorf("cleanup target %s is not the ledger resource; refusing deletion", targetID)
				}
				present = true
			}
		}
		if !present {
			e.State, e.Cleanup, e.UpdatedAt = "cleaned", "complete", time.Now().Unix()
			out = append(out, Result{Profile: e.Profile, Target: e.Name, State: e.State, Cleanup: e.Cleanup, Ledger: ledgerPath})
			continue
		}
		upid, err := d.VMDelete(ctx, e.Node, e.VMID)
		if err != nil {
			e.State, e.Error = "unknown", "cleanup request failed"
			_ = Save(ledgerPath, l)
			return nil, err
		}
		if err := waitTask(ctx, p, e.Node, upid); err != nil {
			e.State, e.Error = "unknown", "cleanup task outcome unavailable"
			_ = Save(ledgerPath, l)
			return nil, err
		}
		vms, err = inspector.VMs(ctx)
		if err != nil {
			return nil, err
		}
		for _, vm := range vms {
			if vm.ID == fmt.Sprintf("%s/%d", e.Node, e.VMID) {
				e.State, e.Error = "unknown", "target still exists"
				_ = Save(ledgerPath, l)
				return nil, fmt.Errorf("cleanup target still exists")
			}
		}
		e.State, e.Cleanup, e.UpdatedAt = "cleaned", "complete", time.Now().Unix()
		out = append(out, Result{Profile: e.Profile, Target: e.Name, State: e.State, Cleanup: e.Cleanup, Ledger: ledgerPath})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no pending ledger entry matched exact confirmation")
	}
	return out, Save(ledgerPath, l)
}
