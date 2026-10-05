package cli

import (
	"encoding/json"
	"testing"

	"github.com/geoffmcc/nodex/internal/domain"
)

// TestLimitIsAlwaysAPrefixOfTheUnlimitedResult reproduces the recorded
// evaluation table. /cluster/resources returned the containers in a different
// order on every call, so --limit 3 was not the first three rows of --limit 0,
// and raising the limit removed one row while re-adding another.
func TestLimitIsAlwaysAPrefixOfTheUnlimitedResult(t *testing.T) {
	// The order the endpoint actually produced on successive calls.
	permutations := [][]string{
		{"proxmox/103", "proxmox/104", "proxmox/102", "proxmox/100", "proxmox/101", "proxmox/105"},
		{"proxmox/100", "proxmox/101", "proxmox/102", "proxmox/103", "proxmox/104", "proxmox/105"},
		{"proxmox/105", "proxmox/104", "proxmox/103", "proxmox/102", "proxmox/101", "proxmox/100"},
	}

	var full []string
	for _, order := range permutations {
		containers := make([]domain.Container, 0, len(order))
		for _, id := range order {
			containers = append(containers, domain.Container{ID: id})
		}

		sortGuestsByID(containers, func(c domain.Container) string { return c.ID })

		if full == nil {
			for _, c := range containers {
				full = append(full, c.ID)
			}
		}

		for limit := 1; limit <= len(order)+2; limit++ {
			page := applyLimit(containers, limit)
			got := make([]string, 0, len(page))
			for _, c := range page {
				got = append(got, c.ID)
			}
			want := len(order)
			if limit < want {
				want = limit
			}
			if len(got) != want {
				t.Fatalf("limit %d returned %d rows, want %d: %v", limit, len(got), want, got)
			}
			for i := range got {
				if got[i] != full[i] {
					t.Fatalf("limit %d row %d = %s, want %s (prefix of %v): %v", limit, i, got[i], full[i], full, got)
				}
			}
		}
	}
}

func TestGuestLessOrdersNumericallyNotLexically(t *testing.T) {
	ids := []string{"proxmox/105", "proxmox/1000", "proxmox/101", "proxmox/99", "proxmox/100"}
	guests := make([]domain.VM, 0, len(ids))
	for _, id := range ids {
		guests = append(guests, domain.VM{ID: id})
	}

	sortGuestsByID(guests, func(v domain.VM) string { return v.ID })

	want := []string{"proxmox/99", "proxmox/100", "proxmox/101", "proxmox/105", "proxmox/1000"}
	for i, id := range want {
		if guests[i].ID != id {
			t.Fatalf("position %d = %s, want %s (full: %v)", i, guests[i].ID, id, guests)
		}
	}
}

// A guest whose identifier does not parse must still land in a stable position
// rather than panicking or being dropped.
func TestGuestLessKeepsUnparsableIdentifiersDeterministically(t *testing.T) {
	ids := []string{"not-a-guest", "proxmox/101", "also-bad", "proxmox/100"}
	guests := make([]domain.VM, 0, len(ids))
	for _, id := range ids {
		guests = append(guests, domain.VM{ID: id})
	}

	sortGuestsByID(guests, func(v domain.VM) string { return v.ID })

	want := []string{"proxmox/100", "proxmox/101", "also-bad", "not-a-guest"}
	for i, id := range want {
		if guests[i].ID != id {
			t.Fatalf("position %d = %s, want %s (full: %v)", i, guests[i].ID, id, guests)
		}
	}
}

func TestSortByFieldOrdersNodesAndStorage(t *testing.T) {
	nodes := []domain.Node{{Name: "pve-b", ID: "node/b"}, {Name: "pve-a", ID: "node/a"}, {Name: "pve-a", ID: "node/0"}, {Name: "pve-c", ID: "node/c"}}
	sortByField(nodes, func(n domain.Node) string { return n.Name + "\x00" + n.ID })
	for i, want := range []string{"node/0", "node/a", "node/b", "node/c"} {
		if nodes[i].ID != want {
			t.Fatalf("node position %d = %s, want %s", i, nodes[i].ID, want)
		}
	}

	storages := []domain.Storage{{Name: "local", ID: "storage/pve-b/local", Node: "pve-b"}, {Name: "backup", ID: "storage/pve-a/backup", Node: "pve-a"}, {Name: "local", ID: "storage/pve-a/local", Node: "pve-a"}, {Name: "local-lvm", ID: "storage/pve-a/local-lvm", Node: "pve-a"}}
	sortByField(storages, func(s domain.Storage) string { return s.Name + "\x00" + s.ID + "\x00" + s.Node })
	for i, want := range []string{"storage/pve-a/backup", "storage/pve-a/local", "storage/pve-b/local", "storage/pve-a/local-lvm"} {
		if storages[i].ID != want {
			t.Fatalf("storage position %d = %s, want %s", i, storages[i].ID, want)
		}
	}
}

// TestLimitMayHaveDroppedRows pins the recorded contradiction: --agent --limit 2
// over 6 containers emitted "truncated": false next to an admission that
// pagination metadata was unavailable, so a caller reading the boolean
// concluded the 2 returned rows were all 6.
func TestLimitMayHaveDroppedRows(t *testing.T) {
	rows := func(n int) json.RawMessage {
		items := make([]map[string]string, 0, n)
		for i := 0; i < n; i++ {
			items = append(items, map[string]string{"id": string(rune('a' + i))})
		}
		b, err := json.Marshal(items)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return b
	}

	tests := []struct {
		name  string
		limit int
		data  json.RawMessage
		want  bool
	}{
		{name: "no limit never reports a withheld row", limit: 0, data: rows(2), want: false},
		{name: "limit reached means rows may have been withheld", limit: 2, data: rows(2), want: true},
		{name: "limit exactly met still may have withheld", limit: 6, data: rows(6), want: true},
		{name: "limit not reached withheld nothing", limit: 10, data: rows(6), want: false},
		{name: "fewer rows than the limit withheld nothing", limit: 2, data: rows(1), want: false},
		{name: "empty data cannot prove a withheld row", limit: 2, data: nil, want: false},
		{name: "empty array withheld nothing", limit: 2, data: rows(0), want: false},
		{name: "object payload cannot prove a withheld row", limit: 2, data: json.RawMessage(`{"items":[]}`), want: false},
		{name: "string payload cannot prove a withheld row", limit: 2, data: json.RawMessage(`"text"`), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := limitMayHaveDroppedRows(tt.limit, tt.data); got != tt.want {
				t.Fatalf("limitMayHaveDroppedRows(%d, %s) = %v, want %v", tt.limit, tt.data, got, tt.want)
			}
		})
	}
}

// The capture bound and an applied --limit are independent reasons the returned
// data is not the whole result; either one alone must reach the boolean, and the
// call site ORs them together.
func TestLimitSignalComposesWithCaptureTruncation(t *testing.T) {
	rows := func(n int) json.RawMessage {
		items := make([]map[string]string, 0, n)
		for i := 0; i < n; i++ {
			items = append(items, map[string]string{"id": "x"})
		}
		b, err := json.Marshal(items)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return b
	}

	// observedTruncated mirrors how agent mode combines the two reasons.
	observedTruncated := func(limit int, raw []byte, captureTruncated bool) bool {
		data, truncated := capturedData(raw, captureTruncated)
		if limitMayHaveDroppedRows(limit, data) {
			truncated = true
		}
		return truncated
	}

	six := rows(6)

	if observedTruncated(0, six, false) {
		t.Error("no limit and an intact capture withheld nothing")
	}
	if !observedTruncated(2, six, false) {
		t.Error("a limit met exactly withholds rows even when the capture is intact")
	}
	if observedTruncated(10, six, false) {
		t.Error("a limit beyond the row count withheld nothing")
	}
	if !observedTruncated(0, six, true) {
		t.Error("a cut-off capture reports truncation on its own")
	}
	if !observedTruncated(10, six, true) {
		t.Error("a cut-off capture reports truncation even when the limit was not reached")
	}
}
