package cli

import (
	"context"
	"strings"
	"testing"
)

// --- vm show runtime status (finding F10) ---
//
// The aggregate VM listing only knows the QEMU process state, so a guest
// suspended in memory reads as "running". vm show must enrich from the
// authoritative per-guest read and report the QMP execution state as its own
// axis, while never inventing one when the read fails.

// TestVMShowReportsQMPExecutionState covers the suspended-in-memory case: the
// process is running while the CPUs are paused. Both axes must appear, and the
// table must gain an EXECUTION row only because the state was observed.
func TestVMShowReportsQMPExecutionState(t *testing.T) {
	isolateConfigAndHome(t)
	setupE2EConfig(t)

	e2eGuestRuntimeQMPOverride = "paused"
	t.Cleanup(func() { e2eGuestRuntimeQMPOverride = "" })

	var out, errOut strings.Builder
	if err := Run(context.Background(), []string{"--output", "json", "vm", "show", "e2e-node/100"}, &out, &errOut); err != nil {
		t.Fatalf("vm show: %v (stderr: %s)", err, errOut.String())
	}
	got := out.String()
	if !strings.Contains(got, `"status": "running"`) {
		t.Errorf("process status must stay running, got: %s", got)
	}
	if !strings.Contains(got, `"qmpstatus": "paused"`) {
		t.Errorf("expected the observed QMP execution state, got: %s", got)
	}

	var tableOut, tableErr strings.Builder
	if err := Run(context.Background(), []string{"--output", "table", "vm", "show", "e2e-node/100"}, &tableOut, &tableErr); err != nil {
		t.Fatalf("vm show (table): %v (stderr: %s)", err, tableErr.String())
	}
	if !strings.Contains(tableOut.String(), "EXECUTION") || !strings.Contains(tableOut.String(), "paused") {
		t.Errorf("table must show the observed execution state, got: %s", tableOut.String())
	}
}

// TestVMShowStoppedGuestHasNoExecutionRow: a stopped VM has no QEMU process
// and therefore no QMP execution state. The field must stay absent rather
// than claim a state that was never observed, and the table must not grow an
// EXECUTION row.
func TestVMShowStoppedGuestHasNoExecutionRow(t *testing.T) {
	isolateConfigAndHome(t)
	setupE2EConfig(t)

	e2eVMStatusOverride = "stopped"
	t.Cleanup(func() { e2eVMStatusOverride = "" })

	var out, errOut strings.Builder
	if err := Run(context.Background(), []string{"--output", "json", "vm", "show", "e2e-node/100"}, &out, &errOut); err != nil {
		t.Fatalf("vm show: %v (stderr: %s)", err, errOut.String())
	}
	got := out.String()
	if !strings.Contains(got, `"status": "stopped"`) {
		t.Errorf("expected the authoritative stopped status, got: %s", got)
	}
	if strings.Contains(got, "qmpstatus") {
		t.Errorf("a stopped guest must not report an execution state, got: %s", got)
	}

	var tableOut, tableErr strings.Builder
	if err := Run(context.Background(), []string{"--output", "table", "vm", "show", "e2e-node/100"}, &tableOut, &tableErr); err != nil {
		t.Fatalf("vm show (table): %v (stderr: %s)", err, tableErr.String())
	}
	if strings.Contains(tableOut.String(), "EXECUTION") {
		t.Errorf("table must not show an unobserved execution state, got: %s", tableOut.String())
	}
}

// TestVMShowRuntimeReadFailureFallsBackToAggregate: when the authoritative
// read fails, the aggregate view is still a truthful statement of what was
// read. It must be kept, the failure must be reported on stderr, and no QMP
// state may be invented.
func TestVMShowRuntimeReadFailureFallsBackToAggregate(t *testing.T) {
	isolateConfigAndHome(t)
	setupE2EConfig(t)

	e2eGuestRuntimeError = 500
	t.Cleanup(func() { e2eGuestRuntimeError = 0 })

	var out, errOut strings.Builder
	if err := Run(context.Background(), []string{"--output", "json", "vm", "show", "e2e-node/100"}, &out, &errOut); err != nil {
		t.Fatalf("vm show: %v (stderr: %s)", err, errOut.String())
	}
	got := out.String()
	if !strings.Contains(got, `"status": "running"`) {
		t.Errorf("aggregate status must be preserved, got: %s", got)
	}
	if strings.Contains(got, "qmpstatus") {
		t.Errorf("no execution state may be invented after a failed read, got: %s", got)
	}
	if !strings.Contains(errOut.String(), "authoritative VM status unavailable") {
		t.Errorf("the failed authoritative read must be reported, got stderr: %s", errOut.String())
	}
}
