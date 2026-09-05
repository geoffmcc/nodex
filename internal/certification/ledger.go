// Package certification contains the deliberately narrow, opt-in production
// certification transaction. It only manages resources bearing the
// nodex-cert- prefix and records cleanup intent before creating anything.
package certification

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/geoffmcc/nodex/internal/atomicwrite"
)

const (
	SchemaVersion = 1
	NamePrefix    = "nodex-cert-"
)

type Entry struct {
	ID        string `json:"id"`
	Profile   string `json:"profile"`
	Node      string `json:"node"`
	VMID      int    `json:"vmid"`
	Name      string `json:"name"`
	Storage   string `json:"storage"`
	State     string `json:"state"`
	Cleanup   string `json:"cleanup"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
	Error     string `json:"error,omitempty"`
}

type Ledger struct {
	Schema  int     `json:"schema"`
	Entries []Entry `json:"entries"`
}

func New(path string) *Ledger { return &Ledger{Schema: SchemaVersion, Entries: []Entry{}} }

func Load(path string) (*Ledger, error) {
	b, err := os.ReadFile(path) // #nosec G304 -- the operator explicitly selects the ledger.
	if err != nil {
		if os.IsNotExist(err) {
			return New(path), nil
		}
		return nil, fmt.Errorf("read certification ledger: %w", err)
	}
	var l Ledger
	if err := json.Unmarshal(b, &l); err != nil {
		return nil, fmt.Errorf("decode certification ledger: %w", err)
	}
	if l.Schema != SchemaVersion {
		return nil, fmt.Errorf("unsupported certification ledger schema %d", l.Schema)
	}
	for _, e := range l.Entries {
		if e.ID == "" || e.Profile == "" || !strings.HasPrefix(e.Name, NamePrefix) || e.VMID <= 0 {
			return nil, fmt.Errorf("invalid certification ledger entry")
		}
	}
	sort.Slice(l.Entries, func(i, j int) bool { return l.Entries[i].ID < l.Entries[j].ID })
	return &l, nil
}

func Save(path string, l *Ledger) error {
	if l == nil || l.Schema != SchemaVersion {
		return fmt.Errorf("invalid certification ledger")
	}
	sort.Slice(l.Entries, func(i, j int) bool { return l.Entries[i].ID < l.Entries[j].ID })
	b, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return atomicwrite.WriteFile(path, append(b, '\n'), true, 0o700, 0o600)
}

func DefaultPath(dir string) string { return filepath.Join(dir, "certification-ledger.json") }

func NewEntry(profile, node string, vmid int, name, storage string, now time.Time) Entry {
	return Entry{ID: fmt.Sprintf("%s%d-%d", NamePrefix, now.UnixNano(), vmid), Profile: profile, Node: node, VMID: vmid, Name: name, Storage: storage, State: "reserved", Cleanup: "required", CreatedAt: now.Unix(), UpdatedAt: now.Unix()}
}
