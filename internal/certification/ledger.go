// Package certification contains the deliberately narrow, opt-in production
// certification transaction. It only manages resources bearing the
// nodex-cert- prefix and records cleanup intent before creating anything.
package certification

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/geoffmcc/nodex/internal/atomicwrite"
	"github.com/geoffmcc/nodex/internal/config"
	"github.com/geoffmcc/nodex/internal/redact"
)

const (
	SchemaVersion        = 2
	legacySchemaVersion  = 1
	NamePrefix           = "nodex-cert-"
	CleanupLeaseDuration = 15 * time.Minute
)

func cleanupLockPath(path string) string { return path + ".cleanup" }

type Entry struct {
	ID               string `json:"id"`
	Profile          string `json:"profile"`
	Node             string `json:"node"`
	VMID             int    `json:"vmid"`
	Name             string `json:"name"`
	Storage          string `json:"storage"`
	State            string `json:"state"`
	Cleanup          string `json:"cleanup"`
	CreatedAt        int64  `json:"created_at"`
	UpdatedAt        int64  `json:"updated_at"`
	Error            string `json:"error,omitempty"`
	Environment      string `json:"environment,omitempty"`
	EndpointIdentity string `json:"endpoint_identity,omitempty"`
	ProviderIdentity string `json:"provider_identity,omitempty"`
	Fingerprint      string `json:"fingerprint,omitempty"`
	CAIdentity       string `json:"ca_identity,omitempty"`
	RunID            string `json:"run_id,omitempty"`
	CreateUPID       string `json:"create_upid,omitempty"`
	DeleteUPID       string `json:"delete_upid,omitempty"`
	Revision         uint64 `json:"revision,omitempty"`
	LeaseID          string `json:"lease_id,omitempty"`
	LeaseExpiresAt   int64  `json:"lease_expires_at,omitempty"`
}

type Ledger struct {
	Schema     int     `json:"schema"`
	Generation uint64  `json:"generation,omitempty"`
	Entries    []Entry `json:"entries"`
	Digest     string  `json:"digest"`
	// LegacyUnverified marks a pre-digest ledger. Its entries may be inspected,
	// but no code may use them to authorize or checkpoint a provider mutation.
	LegacyUnverified bool `json:"-"`
}

// IntegrityStatus reports whether ledger entries carry the current integrity
// check. Legacy ledgers remain readable for reports but are mutation-ineligible.
func (l *Ledger) IntegrityStatus() string {
	if l != nil && l.LegacyUnverified {
		return "legacy_unverified"
	}
	return "verified"
}

// Report is the sanitized, read-only representation returned by the report
// command. The integrity status is computed at load time and is not part of
// the persisted ledger or its digest.
type Report struct {
	Schema          int     `json:"schema" yaml:"schema"`
	Generation      uint64  `json:"generation,omitempty" yaml:"generation,omitempty"`
	Entries         []Entry `json:"entries" yaml:"entries"`
	Digest          string  `json:"digest,omitempty" yaml:"digest,omitempty"`
	IntegrityStatus string  `json:"integrity_status" yaml:"integrity_status"`
}

func (l *Ledger) Report() Report {
	return Report{Schema: l.Schema, Generation: l.Generation, Entries: l.Entries, Digest: l.Digest, IntegrityStatus: l.IntegrityStatus()}
}

func New(path string) *Ledger {
	l := &Ledger{Schema: SchemaVersion, Entries: []Entry{}}
	l.Digest, _ = ledgerDigest(*l)
	return l
}

func Load(path string) (*Ledger, error) {
	lock, err := config.Lock(path)
	if err != nil {
		return nil, fmt.Errorf("lock certification ledger: %w", err)
	}
	defer func() { _ = config.Unlock(lock) }()
	return loadUnlocked(path)
}

func loadUnlocked(path string) (*Ledger, error) {
	b, err := os.ReadFile(path) // #nosec G304 -- the operator explicitly selects the ledger.
	if err != nil {
		if os.IsNotExist(err) {
			return New(path), nil
		}
		return nil, fmt.Errorf("read certification ledger: %w", err)
	}
	return decodeLedger(b)
}

func decodeLedger(b []byte) (*Ledger, error) {
	var l Ledger
	var err error
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&l); err != nil {
		return nil, fmt.Errorf("decode certification ledger: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("certification ledger contains multiple JSON values")
	}
	if l.Schema != SchemaVersion && l.Schema != legacySchemaVersion {
		return nil, fmt.Errorf("unsupported certification ledger schema %d", l.Schema)
	}
	seen := map[string]bool{}
	for _, e := range l.Entries {
		if e.ID == "" || seen[e.ID] || e.Profile == "" || !strings.HasPrefix(e.Name, NamePrefix) || e.VMID <= 0 || e.CreatedAt <= 0 || e.UpdatedAt < e.CreatedAt || !validEntryState(e.State) || !validCleanupState(e.Cleanup) || (e.LeaseID == "" && e.LeaseExpiresAt != 0) || (e.LeaseID != "" && e.LeaseExpiresAt <= 0) {
			return nil, fmt.Errorf("invalid certification ledger entry")
		}
		seen[e.ID] = true
	}
	sort.Slice(l.Entries, func(i, j int) bool { return l.Entries[i].ID < l.Entries[j].ID })
	if l.Digest == "" {
		// Schema 1 predates ledger digests. Preserve populated legacy claims for
		// read-only reporting, but never treat their contents as authority for a
		// provider mutation. A first-run empty ledger carries no claims and can
		// be safely upgraded in memory.
		if l.Schema == legacySchemaVersion && (l.Generation != 0 || len(l.Entries) != 0) {
			l.LegacyUnverified = true
			return &l, nil
		}
		if l.Generation == 0 && l.Entries != nil && len(l.Entries) == 0 {
			l.Schema = SchemaVersion
			l.Digest, err = ledgerDigest(l)
			if err != nil {
				return nil, fmt.Errorf("digest empty certification ledger: %w", err)
			}
			return &l, nil
		}
		return nil, fmt.Errorf("certification ledger has no digest")
	}
	want, err := ledgerDigest(l)
	if err != nil || want != l.Digest {
		return nil, fmt.Errorf("certification ledger digest mismatch")
	}
	return &l, nil
}

func Save(path string, l *Ledger) error {
	lock, err := config.Lock(path)
	if err != nil {
		return fmt.Errorf("lock certification ledger: %w", err)
	}
	defer func() { _ = config.Unlock(lock) }()
	return saveUnlocked(path, l)
}

// MigrateCompletedLegacy upgrades a populated schema 1 ledger only when every
// recorded transaction is conclusively cleaned up. It preserves the exact
// original bytes in a sibling backup before writing the schema 2 digest. The
// backup is never overwritten with different content.
func MigrateCompletedLegacy(path string) (string, int, error) {
	if err := validateMutationLedgerPath(path); err != nil {
		return "", 0, err
	}
	cleanupLock, err := config.Lock(cleanupLockPath(path))
	if err != nil {
		return "", 0, fmt.Errorf("lock certification cleanup: %w", err)
	}
	defer func() { _ = config.Unlock(cleanupLock) }()
	lock, err := config.Lock(path)
	if err != nil {
		return "", 0, fmt.Errorf("lock certification ledger: %w", err)
	}
	defer func() { _ = config.Unlock(lock) }()

	info, err := os.Lstat(path)
	if err != nil {
		return "", 0, fmt.Errorf("inspect certification ledger: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", 0, fmt.Errorf("certification ledger must be a regular file")
	}
	original, err := os.ReadFile(path) // #nosec G304 -- path is restricted to the default per-configuration ledger.
	if err != nil {
		return "", 0, fmt.Errorf("read certification ledger: %w", err)
	}
	l, err := decodeLedger(original)
	if err != nil {
		return "", 0, err
	}
	if !l.LegacyUnverified {
		return "", 0, fmt.Errorf("certification ledger is already verified; no migration is needed")
	}
	for _, entry := range l.Entries {
		if entry.Cleanup != "complete" || entry.LeaseID != "" || entry.LeaseExpiresAt != 0 {
			return "", 0, fmt.Errorf("legacy ledger contains a pending or leased transaction; reconcile it from a trusted source before migration")
		}
	}
	backup := path + ".legacy-v1.bak"
	if backupInfo, statErr := os.Lstat(backup); statErr == nil {
		if !backupInfo.Mode().IsRegular() {
			return "", 0, fmt.Errorf("legacy ledger backup must be a regular file")
		}
		prior, readErr := os.ReadFile(backup)
		if readErr != nil {
			return "", 0, fmt.Errorf("read legacy ledger backup: %w", readErr)
		}
		if !bytes.Equal(prior, original) {
			return "", 0, fmt.Errorf("legacy ledger backup already exists with different contents: %s", backup)
		}
	} else if os.IsNotExist(statErr) {
		if err := atomicwrite.WriteFile(backup, original, false, 0o700, 0o600); err != nil {
			return "", 0, fmt.Errorf("preserve legacy ledger backup: %w", err)
		}
	} else {
		return "", 0, fmt.Errorf("inspect legacy ledger backup: %w", statErr)
	}

	l.Schema = SchemaVersion
	l.LegacyUnverified = false
	if err := saveUnlocked(path, l); err != nil {
		return "", 0, fmt.Errorf("write migrated certification ledger (original preserved at %s): %w", backup, err)
	}
	return backup, len(l.Entries), nil
}

func saveUnlocked(path string, l *Ledger) error {
	if l == nil {
		return fmt.Errorf("invalid certification ledger")
	}
	if err := requireVerifiedLedger(l); err != nil {
		return err
	}
	if l.Schema == legacySchemaVersion {
		if l.Digest == "" && len(l.Entries) != 0 {
			return legacyLedgerMutationError()
		}
		l.Schema = SchemaVersion
	}
	if l.Schema != SchemaVersion {
		return fmt.Errorf("invalid certification ledger")
	}
	sort.Slice(l.Entries, func(i, j int) bool { return l.Entries[i].ID < l.Entries[j].ID })
	seen := map[string]bool{}
	for i := range l.Entries {
		e := &l.Entries[i]
		if e.ID == "" || seen[e.ID] || e.Profile == "" || !strings.HasPrefix(e.Name, NamePrefix) || e.VMID <= 0 || e.CreatedAt <= 0 || e.UpdatedAt < e.CreatedAt || !validEntryState(e.State) || !validCleanupState(e.Cleanup) || (e.LeaseID == "" && e.LeaseExpiresAt != 0) || (e.LeaseID != "" && e.LeaseExpiresAt <= 0) {
			return fmt.Errorf("invalid certification ledger entry")
		}
		e.Error = redact.String(e.Error)
		seen[e.ID] = true
	}
	l.Generation++
	digest, err := ledgerDigest(*l)
	if err != nil {
		return err
	}
	l.Digest = digest
	b, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return atomicwrite.WriteFile(path, append(b, '\n'), true, 0o700, 0o600)
}

// Reserve atomically checks the environment resource cap and records cleanup
// intent before a provider mutation begins.
func Reserve(path string, entry Entry, maxResources int) (*Ledger, error) {
	if maxResources <= 0 {
		return nil, fmt.Errorf("invalid certification resource limit")
	}
	if entry.State != "reserved" || entry.Cleanup != "required" {
		return nil, fmt.Errorf("new certification entries must start reserved with cleanup required")
	}
	lock, err := config.Lock(path)
	if err != nil {
		return nil, fmt.Errorf("lock certification ledger: %w", err)
	}
	defer func() { _ = config.Unlock(lock) }()
	l, err := loadUnlocked(path)
	if err != nil {
		return nil, err
	}
	if err := requireVerifiedLedger(l); err != nil {
		return nil, err
	}
	active := 0
	for _, prior := range l.Entries {
		if prior.Environment == entry.Environment && prior.Cleanup != "complete" {
			active++
		}
		if prior.Name == entry.Name || (prior.VMID == entry.VMID && prior.Node == entry.Node) {
			return nil, fmt.Errorf("certification target is already present in the cleanup ledger")
		}
	}
	if active >= maxResources {
		return nil, fmt.Errorf("certification resource limit reached for environment %q", entry.Environment)
	}
	leaseID, err := newLeaseID()
	if err != nil {
		return nil, err
	}
	entry.LeaseID = leaseID
	entry.LeaseExpiresAt = time.Now().Add(CleanupLeaseDuration).Unix()
	if entry.Revision == 0 {
		entry.Revision = 1
	}
	l.Entries = append(l.Entries, entry)
	if err := saveUnlocked(path, l); err != nil {
		return nil, err
	}
	return l, nil
}

func ClaimCleanup(path, id string, auth *Authorization) (Entry, error) {
	lock, err := config.Lock(cleanupLockPath(path))
	if err != nil {
		return Entry{}, fmt.Errorf("lock certification cleanup: %w", err)
	}
	defer func() { _ = config.Unlock(lock) }()
	return claimCleanup(path, id, auth)
}

// claimCleanup performs the ledger-only portion of cleanup claiming. Callers
// that will perform provider mutations must hold the cleanup lock for the
// entire transaction so an expired worker cannot overlap recovery.
func claimCleanup(path, id string, auth *Authorization) (Entry, error) {
	if err := validateAuthorizationBinding(auth); err != nil {
		return Entry{}, err
	}
	lock, err := config.Lock(path)
	if err != nil {
		return Entry{}, fmt.Errorf("lock certification ledger: %w", err)
	}
	defer func() { _ = config.Unlock(lock) }()
	l, err := loadUnlocked(path)
	if err != nil {
		return Entry{}, err
	}
	if err := requireVerifiedLedger(l); err != nil {
		return Entry{}, err
	}
	for i := range l.Entries {
		e := &l.Entries[i]
		if e.ID != id {
			continue
		}
		if e.Cleanup == "complete" {
			return Entry{}, fmt.Errorf("no pending ledger entry matched exact confirmation")
		}
		if e.Profile == "" || !strings.HasPrefix(e.Name, NamePrefix) {
			return Entry{}, fmt.Errorf("ledger entry is outside certification safety boundary")
		}
		if e.Environment != auth.Environment || e.Profile != auth.Profile || e.EndpointIdentity != auth.Endpoint || !strings.EqualFold(e.ProviderIdentity, auth.Provider) || e.Fingerprint != auth.ExpectedFingerprint || e.CAIdentity != auth.TrustedCAIdentity {
			return Entry{}, fmt.Errorf("ledger entry does not match the current certification authorization")
		}
		now := time.Now().Unix()
		if auth.ExpiresAt <= now {
			return Entry{}, fmt.Errorf("certification cleanup authorization is expired")
		}
		if !auth.AllowMutations || !contains(auth.Suites, "disposable-mutations") || !contains(auth.Nodes, e.Node) || !contains(auth.Storage, e.Storage) || e.VMID < auth.VMIDMin || e.VMID > auth.VMIDMax {
			return Entry{}, fmt.Errorf("certification cleanup target is outside the authorized resource range")
		}
		if (e.State == "reserved" || e.State == "creating") && (e.LeaseID == "" || e.LeaseExpiresAt > now) {
			return Entry{}, fmt.Errorf("certification creation is still in progress")
		}
		if e.Cleanup == "in_progress" && e.LeaseID != "" && e.LeaseExpiresAt > now {
			return Entry{}, fmt.Errorf("certification cleanup is already leased until %s", time.Unix(e.LeaseExpiresAt, 0).UTC().Format(time.RFC3339))
		}
		leaseID, leaseErr := newLeaseID()
		if leaseErr != nil {
			return Entry{}, leaseErr
		}
		e.State, e.Cleanup, e.UpdatedAt = "deleting", "in_progress", now
		e.LeaseID, e.LeaseExpiresAt = leaseID, time.Now().Add(CleanupLeaseDuration).Unix()
		e.Revision++
		if err := saveUnlocked(path, l); err != nil {
			return Entry{}, fmt.Errorf("checkpoint cleanup: %w", err)
		}
		return *e, nil
	}
	return Entry{}, fmt.Errorf("no pending ledger entry matched exact confirmation")
}

func verifyLease(path, id, leaseID string, expectedRevision uint64) error {
	if leaseID == "" {
		return fmt.Errorf("certification cleanup lease is missing")
	}
	lock, err := config.Lock(path)
	if err != nil {
		return fmt.Errorf("lock certification ledger: %w", err)
	}
	defer func() { _ = config.Unlock(lock) }()
	l, err := loadUnlocked(path)
	if err != nil {
		return err
	}
	if err := requireVerifiedLedger(l); err != nil {
		return err
	}
	for _, entry := range l.Entries {
		if entry.ID != id {
			continue
		}
		if entry.LeaseID != leaseID {
			return fmt.Errorf("ledger entry %q lease is no longer current", id)
		}
		if entry.LeaseExpiresAt <= time.Now().Unix() {
			return fmt.Errorf("ledger entry %q lease has expired", id)
		}
		if entry.Revision != expectedRevision {
			return fmt.Errorf("ledger entry %q revision is no longer current", id)
		}
		return nil
	}
	return fmt.Errorf("ledger entry %q not found", id)
}

func UpdateEntry(path, id string, update func(*Entry)) error {
	lock, err := config.Lock(path)
	if err != nil {
		return fmt.Errorf("lock certification ledger: %w", err)
	}
	defer func() { _ = config.Unlock(lock) }()
	l, err := loadUnlocked(path)
	if err != nil {
		return err
	}
	if err := requireVerifiedLedger(l); err != nil {
		return err
	}
	for i := range l.Entries {
		if l.Entries[i].ID == id {
			if l.Entries[i].LeaseID != "" {
				return fmt.Errorf("ledger entry %q is lease-protected; lease token required", id)
			}
			if update == nil {
				return fmt.Errorf("ledger entry update is nil")
			}
			update(&l.Entries[i])
			l.Entries[i].Revision++
			return saveUnlocked(path, l)
		}
	}
	return fmt.Errorf("ledger entry %q not found", id)
}

// UpdateEntryWithLease applies a checkpoint only when the caller still owns
// the current fencing token and has the current entry revision. A recovered
// cleanup gets a new token, so an old worker cannot overwrite the recovered
// state after its lease expires.
func UpdateEntryWithLease(path, id, leaseID string, expectedRevision uint64, update func(*Entry)) error {
	if leaseID == "" || update == nil {
		return fmt.Errorf("a lease token and update are required")
	}
	lock, err := config.Lock(path)
	if err != nil {
		return fmt.Errorf("lock certification ledger: %w", err)
	}
	defer func() { _ = config.Unlock(lock) }()
	l, err := loadUnlocked(path)
	if err != nil {
		return err
	}
	for i := range l.Entries {
		e := &l.Entries[i]
		if e.ID != id {
			continue
		}
		if e.LeaseID != leaseID {
			return fmt.Errorf("ledger entry %q lease is no longer current", id)
		}
		if e.LeaseExpiresAt > 0 && e.LeaseExpiresAt <= time.Now().Unix() {
			return fmt.Errorf("ledger entry %q lease has expired", id)
		}
		if e.Revision != expectedRevision {
			return fmt.Errorf("ledger entry %q revision is no longer current", id)
		}
		update(e)
		e.Revision = expectedRevision + 1
		return saveUnlocked(path, l)
	}
	return fmt.Errorf("ledger entry %q not found", id)
}

// RenewLease extends a cleanup lease without changing the resource state.
func RenewLease(path, id, leaseID string, expectedRevision uint64, now time.Time) error {
	return UpdateEntryWithLease(path, id, leaseID, expectedRevision, func(e *Entry) {
		e.LeaseExpiresAt = now.Add(CleanupLeaseDuration).Unix()
		e.UpdatedAt = now.Unix()
	})
}

func requireVerifiedLedger(l *Ledger) error {
	if l != nil && l.LegacyUnverified {
		return legacyLedgerMutationError()
	}
	return nil
}

func legacyLedgerMutationError() error {
	return fmt.Errorf("legacy certification ledger has unauthenticated claims; mutations are blocked until it is restored or reconciled from a trusted source")
}

func newLeaseID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate certification lease: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func ledgerDigest(l Ledger) (string, error) {
	l.Digest = ""
	b, err := json.Marshal(l)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}
func validEntryState(s string) bool {
	switch s {
	case "reserved", "creating", "created", "deleting", "succeeded", "cleaned", "failed", "unknown":
		return true
	}
	return false
}
func validCleanupState(s string) bool {
	switch s {
	case "required", "in_progress", "complete":
		return true
	}
	return false
}

func DefaultPath(dir string) string { return filepath.Join(dir, "certification-ledger.json") }

func NewEntry(profile, node string, vmid int, name, storage string, now time.Time) Entry {
	return Entry{ID: fmt.Sprintf("%s%d-%d", NamePrefix, now.UnixNano(), vmid), Profile: profile, Node: node, VMID: vmid, Name: name, Storage: storage, State: "reserved", Cleanup: "required", CreatedAt: now.Unix(), UpdatedAt: now.Unix()}
}
