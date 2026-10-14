package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/output"
)

// A modern systemd-calendar schedule is accepted on its own.
func TestParseBackupScheduleArgsAcceptsCalendarSchedule(t *testing.T) {
	params, err := parseBackupScheduleArgs([]string{"storage=pbs", "mode=snapshot", "schedule=mon..fri 03:30"})
	if err != nil {
		t.Fatalf("parseBackupScheduleArgs: %v", err)
	}
	if params.Schedule != "mon..fri 03:30" {
		t.Errorf("schedule = %q, want %q", params.Schedule, "mon..fri 03:30")
	}
	if params.Starttime != "" {
		t.Errorf("starttime = %q, want empty", params.Starttime)
	}
}

// A legacy starttime/dow pair is still accepted for older clusters.
func TestParseBackupScheduleArgsAcceptsLegacyStarttime(t *testing.T) {
	params, err := parseBackupScheduleArgs([]string{"storage=local", "mode=suspend", "starttime=01:15", "dow=sun"})
	if err != nil {
		t.Fatalf("parseBackupScheduleArgs: %v", err)
	}
	if params.Starttime != "01:15" || params.Dow != "sun" {
		t.Errorf("starttime/dow = %q/%q, want 01:15/sun", params.Starttime, params.Dow)
	}
}

// Mixing the deprecated pair with the modern calendar form is a usage error
// rather than a request PVE would reject remotely or silently normalize.
func TestParseBackupScheduleArgsRejectsScheduleAndStarttimeTogether(t *testing.T) {
	_, err := parseBackupScheduleArgs([]string{"storage=pbs", "mode=snapshot", "schedule=sun 04:00", "starttime=04:00"})
	if err == nil {
		t.Fatal("expected a mutual-exclusivity error")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error = %v, want mention of mutual exclusivity", err)
	}
}

// A malformed clock time is caught locally before any provider round trip.
func TestParseBackupScheduleArgsRejectsMalformedStarttime(t *testing.T) {
	if _, err := parseBackupScheduleArgs([]string{"storage=pbs", "mode=snapshot", "starttime=3am"}); err == nil {
		t.Fatal("expected an error for a malformed starttime")
	}
}

// The schedule column shows the modern calendar when present and falls back to
// the legacy dow/starttime pair for older jobs.
func TestBackupScheduleTimingRendering(t *testing.T) {
	if got := backupScheduleTiming(domain.BackupSchedule{Schedule: "mon..fri 03:30"}); got != "mon..fri 03:30" {
		t.Errorf("modern = %q", got)
	}
	if got := backupScheduleTiming(domain.BackupSchedule{Dow: "sun", Starttime: "01:15"}); got != "sun 01:15" {
		t.Errorf("legacy = %q, want %q", got, "sun 01:15")
	}
	if got := backupScheduleTiming(domain.BackupSchedule{Starttime: "01:15"}); got != "01:15" {
		t.Errorf("starttime-only = %q, want %q", got, "01:15")
	}
}

// The list table exposes the schedule and next-run columns.
func TestWriteBackupSchedulesTableShowsScheduleAndNextRun(t *testing.T) {
	var stdout bytes.Buffer
	cmdCtx := &Context{Writer: &stdout, Opts: Options{Output: output.FormatTable}}
	schedules := []domain.BackupSchedule{
		{ID: "modern", Storage: "pbs", Mode: "snapshot", Schedule: "mon..fri 03:30", NextRun: 1760500000, Enabled: 1},
		{ID: "legacy", Storage: "local", Mode: "suspend", Starttime: "01:15", Dow: "sun"},
	}
	if err := writeBackupSchedules(cmdCtx, schedules); err != nil {
		t.Fatalf("writeBackupSchedules: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{"SCHEDULE", "NEXT", "mon..fri 03:30", "sun 01:15"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q:\n%s", want, out)
		}
	}
}
