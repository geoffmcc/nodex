package cli

import (
	"testing"
	"time"

	"github.com/geoffmcc/nodex/internal/domain"
)

func TestNodeTimeLocalHumanRendersInConfiguredZone(t *testing.T) {
	// 1784073342 is a true UTC epoch. Rendering it in a UTC-4 zone must apply
	// the offset exactly once, not twice.
	nt := &domain.NodeTime{TimeZone: "America/New_York", Epoch: 1784073342}
	got := nodeTimeLocalHuman(nt)
	want := time.Unix(1784073342, 0).In(mustLoadLocation(t, "America/New_York")).Format(time.RFC3339)
	if got != want {
		t.Errorf("local_human = %q, want %q", got, want)
	}
	if got == "" {
		t.Error("local_human must render for a valid embedded IANA zone")
	}
}

// TestNodeTimeLocalHumanRejectsUnknownZone covers the fail-loudly requirement.
// Falling back to UTC would present a wrong-but-plausible local time, so an
// unloadable zone yields an empty field the caller can detect.
func TestNodeTimeLocalHumanRejectsUnknownZone(t *testing.T) {
	nt := &domain.NodeTime{TimeZone: "Not/AZone", Epoch: 1784073342}
	if got := nodeTimeLocalHuman(nt); got != "" {
		t.Errorf("local_human = %q, want empty for an unknown zone", got)
	}
}

func TestNodeTimeLocalHumanEmptyInputs(t *testing.T) {
	if got := nodeTimeLocalHuman(nil); got != "" {
		t.Errorf("nil node time = %q, want empty", got)
	}
	if got := nodeTimeLocalHuman(&domain.NodeTime{TimeZone: "UTC"}); got != "" {
		t.Errorf("zero epoch = %q, want empty", got)
	}
}

func mustLoadLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("time.LoadLocation(%q) failed: %v; is time/tzdata embedded?", name, err)
	}
	return loc
}
