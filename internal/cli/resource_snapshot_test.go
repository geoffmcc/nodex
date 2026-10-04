package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/domain"
)

func TestFormatSnapshotTime(t *testing.T) {
	cases := []struct {
		name  string
		epoch int
		want  string
	}{
		{name: "rendered as UTC RFC3339", epoch: 1790869735, want: "2026-10-01T15:48:55Z"},
		// The Proxmox VE "current" pseudo-snapshot carries no creation time.
		// Rendering it as the Unix epoch would be a false claim, so it is blank.
		{name: "unset stays blank", epoch: 0, want: ""},
		{name: "negative stays blank", epoch: -1, want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatSnapshotTime(tc.epoch); got != tc.want {
				t.Fatalf("formatSnapshotTime(%d) = %q, want %q", tc.epoch, got, tc.want)
			}
		})
	}
}

// The snapshot list table must surface the creation time and description that
// the provider now preserves, instead of a bare epoch integer and no description.
func TestWriteSnapshotListTableShowsTimeAndDescription(t *testing.T) {
	var buf bytes.Buffer
	cmdCtx := &Context{Writer: &buf}
	snaps := []domain.Snapshot{
		{Name: "nx4-desc", Parent: "nx4-base", Ctime: 1790869735, Description: "marker text"},
		{Name: "current", Parent: "nx4-desc", Description: "You are here!"},
	}

	if err := writeSnapshotList(cmdCtx, snaps); err != nil {
		t.Fatalf("writeSnapshotList: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"NAME", "PARENT", "CREATED", "DESCRIPTION",
		"nx4-desc", "nx4-base", "2026-10-01T15:48:55Z", "marker text",
		"current", "You are here!",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("snapshot table missing %q; got:\n%s", want, out)
		}
	}
	// The raw epoch must no longer be printed as the created value.
	if strings.Contains(out, "1790869735") {
		t.Errorf("snapshot table still prints the raw epoch; got:\n%s", out)
	}
}
