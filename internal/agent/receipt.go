// Package agent defines the versioned machine result and local mutation ledger
// used by NodeX's opt-in agent interface.
package agent

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/geoffmcc/nodex/internal/atomicwrite"
	"github.com/geoffmcc/nodex/internal/config"
)

const (
	// ResultSchemaVersion versions both returned results and durable receipts.
	ResultSchemaVersion = 1
	maxReceiptBytes     = 1 << 20
	maxReceiptFiles     = 10000
	maxReceiptList      = 100
)

var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)
var warningCodePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

const fingerprintKeyBytes = 32

type Submission string
type Execution string
type Verification string
type Retry string

const (
	SubmissionNotAttempted Submission = "not_attempted"
	SubmissionAccepted     Submission = "accepted"
	SubmissionRejected     Submission = "rejected"
	SubmissionUnknown      Submission = "unknown"

	ExecutionNotStarted Execution = "not_started"
	ExecutionRunning    Execution = "running"
	ExecutionSucceeded  Execution = "succeeded"
	ExecutionFailed     Execution = "failed"
	ExecutionUnknown    Execution = "unknown"

	VerificationNotRequested Verification = "not_requested"
	VerificationPassed       Verification = "passed"
	VerificationFailed       Verification = "failed"
	VerificationUnknown      Verification = "unknown"
	VerificationUnsupported  Verification = "unsupported"

	RetrySafe           Retry = "safe"
	RetryReconcileFirst Retry = "reconcile_first"
	RetryDoNotAutomatic Retry = "do_not_retry_automatically"
)

// ExecutionContext binds an operation to the selected profile and its actual
// provider endpoint. Resource IDs are meaningful only inside that identity.
type ExecutionContext struct {
	Profile          string           `json:"profile,omitempty"`
	Provider         string           `json:"provider,omitempty"`
	Endpoint         string           `json:"endpoint,omitempty"`
	ResourceType     string           `json:"resource_type,omitempty"`
	ResourceID       string           `json:"resource_id,omitempty"`
	RelatedTargets   []ResourceTarget `json:"related_targets,omitempty"`
	Node             string           `json:"node,omitempty"`
	Namespace        string           `json:"namespace,omitempty"`
	EndpointIdentity string           `json:"endpoint_identity,omitempty"`
	TLSCAFile        string           `json:"tls_ca_file,omitempty"`
	SSHHost          string           `json:"ssh_host,omitempty"`
	SSHUser          string           `json:"ssh_user,omitempty"`
	SSHKeyFile       string           `json:"ssh_key_file,omitempty"`
	SSHKnownHosts    string           `json:"ssh_known_hosts_file,omitempty"`
	SSHInventoryHost string           `json:"ssh_inventory_host,omitempty"`
	SSHPort          int              `json:"ssh_port,omitempty"`
}

type ResourceTarget struct {
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	Node         string `json:"node,omitempty"`
	Namespace    string `json:"namespace,omitempty"`
}

type AgentError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Exit    int    `json:"exit_code"`
}

// NextAction contains only a canonical NodeX operation identifier and typed
// arguments. Provider-controlled messages are never executable instructions.
type NextAction struct {
	Operation string         `json:"operation"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Observation struct {
	At           string   `json:"at"`
	Completeness string   `json:"completeness"`
	Truncated    bool     `json:"truncated"`
	Partial      bool     `json:"partial"`
	Unsupported  []string `json:"unsupported,omitempty"`
}

// Result is returned for every request made in agent mode. Data is explicitly
// page/provider-derived and must be treated as untrusted input.
type Result struct {
	SchemaVersion int              `json:"schema_version"`
	RequestID     string           `json:"request_id"`
	ReceiptID     string           `json:"receipt_id,omitempty"`
	Operation     string           `json:"operation"`
	Context       ExecutionContext `json:"context"`
	Submission    Submission       `json:"submission"`
	Execution     Execution        `json:"execution"`
	Verification  Verification     `json:"verification"`
	Changed       *bool            `json:"changed"`
	Retry         Retry            `json:"retry"`
	TaskID        string           `json:"task_id,omitempty"`
	StartedAt     string           `json:"started_at"`
	ObservedAt    string           `json:"observed_at"`
	Error         *AgentError      `json:"error,omitempty"`
	Warnings      []Warning        `json:"warnings,omitempty"`
	NextActions   []NextAction     `json:"next_actions,omitempty"`
	Observation   *Observation     `json:"observation,omitempty"`
	Data          json.RawMessage  `json:"data,omitempty"`
}

// Receipt adds only the normalized-input fingerprint and durable ledger
// metadata to a Result. It never contains command strings, credentials, or
// unbounded provider response bodies.
type Receipt struct {
	Result
	InputFingerprint string `json:"input_fingerprint"`
	UpdatedAt        string `json:"updated_at"`
}

func NewRequestID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate request ID: %w", err)
	}
	return "req_" + hex.EncodeToString(b), nil
}

func ValidRequestID(id string) bool { return requestIDPattern.MatchString(id) }

func fingerprintWithKey(key []byte, operation string, args []string, executionContext ExecutionContext) (string, error) {
	canonical := struct {
		Operation string           `json:"operation"`
		Args      []string         `json:"args"`
		Context   ExecutionContext `json:"context"`
	}{operation, append([]string(nil), args...), executionContext}
	b, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("normalize request: %w", err)
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(append([]byte("nodex-agent-input-v1\x00"), b...))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func (r Result) Validate() error {
	if r.SchemaVersion != ResultSchemaVersion || !ValidRequestID(r.RequestID) || strings.TrimSpace(r.Operation) == "" {
		return errors.New("invalid agent result identity or schema")
	}
	if !validSubmission(r.Submission) || !validExecution(r.Execution) || !validVerification(r.Verification) || !validRetry(r.Retry) {
		return errors.New("invalid agent result state")
	}
	if r.StartedAt == "" || r.ObservedAt == "" {
		return errors.New("agent result timestamps are required")
	}
	if _, err := time.Parse(time.RFC3339Nano, r.StartedAt); err != nil {
		return errors.New("invalid agent result started_at")
	}
	if _, err := time.Parse(time.RFC3339Nano, r.ObservedAt); err != nil {
		return errors.New("invalid agent result observed_at")
	}
	startedAt, _ := time.Parse(time.RFC3339Nano, r.StartedAt)
	observedAt, _ := time.Parse(time.RFC3339Nano, r.ObservedAt)
	if observedAt.Before(startedAt) {
		return errors.New("agent result observed_at precedes started_at")
	}
	if r.Submission == SubmissionNotAttempted && r.Execution != ExecutionNotStarted && r.Execution != ExecutionSucceeded && r.Execution != ExecutionFailed {
		return errors.New("not-attempted submission has an incompatible execution state")
	}
	if r.Submission == SubmissionRejected && r.Execution != ExecutionNotStarted {
		return errors.New("rejected submission must not have started execution")
	}
	if r.Submission == SubmissionAccepted && r.Execution == ExecutionNotStarted {
		return errors.New("accepted submission must have an execution state")
	}
	if r.Submission == SubmissionUnknown && r.Execution != ExecutionUnknown && r.Execution != ExecutionRunning {
		return errors.New("unknown submission must retain an unknown or running execution state")
	}
	if r.Submission == SubmissionUnknown && r.Retry != RetryReconcileFirst {
		return errors.New("unknown submission must be reconciled before retry")
	}
	if r.Submission == SubmissionAccepted && (r.Execution == ExecutionRunning || r.Execution == ExecutionUnknown) {
		// An accepted request whose completion cannot be observed has two
		// honest options. Reconcile first when provider evidence could settle
		// it, or do not retry automatically when that evidence does not exist
		// and reconciliation can never succeed for this receipt. The second is
		// strictly more conservative, so allowing it cannot license an unsafe
		// resubmission; it only stops the receipt from pointing a caller at a
		// recovery step that is known to be unavailable.
		if r.Retry != RetryReconcileFirst && r.Retry != RetryDoNotAutomatic {
			return errors.New("accepted-but-unresolved operation must be reconciled before retry or marked as not automatically retryable")
		}
	}
	if r.Verification == VerificationPassed && r.Execution != ExecutionSucceeded {
		return errors.New("verification cannot pass before execution succeeds")
	}
	if r.Verification == VerificationFailed && r.Execution != ExecutionSucceeded {
		return errors.New("postcondition verification failure requires a completed execution")
	}
	if r.Verification == VerificationFailed && r.Submission != SubmissionAccepted {
		return errors.New("postcondition verification failure requires an accepted mutation")
	}
	if r.Submission == SubmissionAccepted && (r.Execution == ExecutionSucceeded || r.Execution == ExecutionFailed) && r.Retry != RetryDoNotAutomatic {
		return errors.New("terminal accepted execution must not be retried automatically")
	}
	if r.Execution == ExecutionFailed && r.Retry != RetryDoNotAutomatic {
		return errors.New("failed execution must not be retried automatically")
	}
	if r.Changed != nil && *r.Changed && r.Submission != SubmissionAccepted {
		return errors.New("change cannot be confirmed without an accepted submission")
	}
	if len(r.Context.RelatedTargets) > 32 {
		return errors.New("agent result related target set exceeds its size limit")
	}
	for _, target := range r.Context.RelatedTargets {
		if strings.TrimSpace(target.ResourceType) == "" || strings.TrimSpace(target.ResourceID) == "" || len(target.ResourceType) > 128 || len(target.ResourceID) > 4096 {
			return errors.New("agent result contains an invalid related target")
		}
	}
	if len(r.Data) > 0 && !json.Valid(r.Data) {
		return errors.New("agent result data is invalid JSON")
	}
	if len(r.TaskID) > 1024 || strings.ContainsAny(r.TaskID, "\x00\r\n\x1b") {
		return errors.New("provider task ID is malformed or oversized")
	}
	if len(r.Warnings) > 64 {
		return errors.New("agent result contains too many warnings")
	}
	for _, warning := range r.Warnings {
		if !warningCodePattern.MatchString(warning.Code) || len([]rune(warning.Message)) > 4200 {
			return errors.New("agent result contains an invalid warning")
		}
	}
	return nil
}

func (r Receipt) Validate() error {
	if err := r.Result.Validate(); err != nil {
		return err
	}
	if r.ReceiptID != r.RequestID || !ValidRequestID(r.ReceiptID) || len(r.InputFingerprint) != 64 {
		return errors.New("invalid receipt identity or fingerprint")
	}
	if _, err := hex.DecodeString(r.InputFingerprint); err != nil {
		return errors.New("invalid receipt fingerprint")
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, r.UpdatedAt)
	if err != nil {
		return errors.New("invalid receipt updated_at")
	}
	observedAt, _ := time.Parse(time.RFC3339Nano, r.ObservedAt)
	if updatedAt.Before(observedAt) {
		return errors.New("receipt updated_at precedes its latest observation")
	}
	if !receiptState(r.Submission, r.Execution) {
		return errors.New("invalid receipt outcome combination")
	}
	return nil
}

func receiptState(submission Submission, execution Execution) bool {
	switch submission {
	case SubmissionNotAttempted:
		return execution == ExecutionNotStarted || execution == ExecutionSucceeded || execution == ExecutionFailed
	case SubmissionRejected:
		return execution == ExecutionNotStarted
	case SubmissionAccepted:
		return execution == ExecutionRunning || execution == ExecutionSucceeded || execution == ExecutionFailed || execution == ExecutionUnknown
	case SubmissionUnknown:
		return execution == ExecutionUnknown || execution == ExecutionRunning
	default:
		return false
	}
}

func validSubmission(v Submission) bool {
	return v == SubmissionNotAttempted || v == SubmissionAccepted || v == SubmissionRejected || v == SubmissionUnknown
}
func validExecution(v Execution) bool {
	return v == ExecutionNotStarted || v == ExecutionRunning || v == ExecutionSucceeded || v == ExecutionFailed || v == ExecutionUnknown
}
func validVerification(v Verification) bool {
	return v == VerificationNotRequested || v == VerificationPassed || v == VerificationFailed || v == VerificationUnknown || v == VerificationUnsupported
}
func validRetry(v Retry) bool {
	return v == RetrySafe || v == RetryReconcileFirst || v == RetryDoNotAutomatic
}

// Store is a local, per-user receipt directory. It is deliberately not a trust
// or authorization store: receipts may be edited by their owner.
type Store struct{ dir string }

func NewStore(dir string) *Store { return &Store{dir: dir} }

func DefaultStore() (*Store, error) {
	dir, err := config.Dir()
	if err != nil {
		return nil, fmt.Errorf("resolve configuration directory: %w", err)
	}
	return NewStore(filepath.Join(dir, "agent-receipts")), nil
}

// Fingerprint computes a stable keyed fingerprint for normalized input. The
// random local key prevents a copied receipt fingerprint from acting as an
// offline dictionary oracle for low-entropy free-text arguments.
func (s *Store) Fingerprint(operation string, args []string, executionContext ExecutionContext) (string, error) {
	key, err := s.fingerprintKey()
	if err != nil {
		return "", err
	}
	return fingerprintWithKey(key, operation, args, executionContext)
}

func (s *Store) fingerprintKey() ([]byte, error) {
	if err := s.ensureDir(); err != nil {
		return nil, err
	}
	path := filepath.Join(s.dir, "fingerprint.key")
	if err := rejectSymlink(path); err != nil {
		return nil, err
	}
	if err := rejectSymlink(path + ".lock"); err != nil {
		return nil, err
	}
	lock, err := config.Lock(path)
	if err != nil {
		return nil, fmt.Errorf("lock fingerprint key: %w", err)
	}
	defer func() { _ = config.Unlock(lock) }()
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		key := make([]byte, fingerprintKeyBytes)
		if _, err := rand.Read(key); err != nil {
			return nil, fmt.Errorf("generate fingerprint key: %w", err)
		}
		if err := atomicwrite.WriteFile(path, key, false, 0o700, 0o600); err != nil {
			return nil, fmt.Errorf("persist fingerprint key: %w", err)
		}
		if err := syncReceiptDirectory(s.dir); err != nil {
			return nil, err
		}
		return key, nil
	}
	if err != nil {
		return nil, fmt.Errorf("inspect fingerprint key: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() != fingerprintKeyBytes {
		return nil, errors.New("fingerprint key is not a bounded regular file")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("fingerprint key permissions are broader than 0600")
	}
	if err := checkReceiptOwner(info); err != nil {
		return nil, err
	}
	key, err := os.ReadFile(path) // #nosec G304 -- path is rooted in the private receipt directory.
	if err != nil {
		return nil, fmt.Errorf("read fingerprint key: %w", err)
	}
	if len(key) != fingerprintKeyBytes {
		return nil, errors.New("fingerprint key has an invalid length")
	}
	return key, nil
}

type Lease struct {
	store *Store
	id    string
	path  string
	lock  *os.File
}

func (s *Store) Lock(id string) (*Lease, error) {
	if s == nil || !ValidRequestID(id) {
		return nil, errors.New("invalid receipt identifier")
	}
	if err := s.ensureDir(); err != nil {
		return nil, err
	}
	path := filepath.Join(s.dir, id+".json")
	if err := rejectSymlink(path); err != nil {
		return nil, err
	}
	if err := rejectSymlink(path + ".lock"); err != nil {
		return nil, err
	}
	lock, err := config.Lock(path)
	if err != nil {
		return nil, fmt.Errorf("lock receipt: %w", err)
	}
	return &Lease{store: s, id: id, path: path, lock: lock}, nil
}

func (l *Lease) Close() error { return config.Unlock(l.lock) }

func (l *Lease) Load() (Receipt, error) {
	if err := l.store.ensureDir(); err != nil {
		return Receipt{}, err
	}
	if err := rejectSymlink(l.path); err != nil {
		return Receipt{}, err
	}
	info, err := os.Lstat(l.path)
	if err != nil {
		return Receipt{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxReceiptBytes {
		return Receipt{}, errors.New("receipt is not a bounded regular file")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return Receipt{}, errors.New("receipt permissions are broader than 0600")
	}
	if err := checkReceiptOwner(info); err != nil {
		return Receipt{}, err
	}
	b, err := os.ReadFile(l.path) // #nosec G304 -- path is rooted in the private receipt directory and ID is validated.
	if err != nil {
		return Receipt{}, fmt.Errorf("read receipt: %w", err)
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	var r Receipt
	if err := dec.Decode(&r); err != nil {
		return Receipt{}, fmt.Errorf("decode receipt: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return Receipt{}, errors.New("receipt contains trailing data")
	}
	if err := r.Validate(); err != nil {
		return Receipt{}, fmt.Errorf("validate receipt: %w", err)
	}
	if r.ReceiptID != l.id {
		return Receipt{}, errors.New("receipt ID does not match its file name")
	}
	return r, nil
}

func (l *Lease) Save(r Receipt) error {
	if r.ReceiptID != l.id {
		return errors.New("receipt ID does not match its lock")
	}
	if err := l.store.ensureDir(); err != nil {
		return err
	}
	if err := rejectSymlink(l.path); err != nil {
		return err
	}
	if err := r.Validate(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if len(b) > maxReceiptBytes {
		return errors.New("receipt exceeds size limit")
	}
	if err := atomicwrite.WriteFile(l.path, append(b, '\n'), true, 0o700, 0o600); err != nil {
		return err
	}
	return syncReceiptDirectory(l.store.dir)
}

func (s *Store) Get(id string) (Receipt, error) {
	l, err := s.Lock(id)
	if err != nil {
		return Receipt{}, err
	}
	defer func() { _ = l.Close() }()
	return l.Load()
}

func (s *Store) List(limit int) ([]Receipt, error) {
	receipts, _, err := s.ListRecent(limit)
	return receipts, err
}

// ListRecent returns at most limit newest receipts and the total number of
// receipt files in the directory. It examines directory metadata first, so a
// short recent list does not read every historical receipt body.
func (s *Store) ListRecent(limit int) ([]Receipt, int, error) {
	if err := s.ensureDir(); err != nil {
		return nil, 0, err
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, 0, err
	}
	if len(entries) > maxReceiptFiles {
		return nil, 0, errors.New("receipt directory exceeds entry limit")
	}
	type candidate struct {
		id      string
		updated time.Time
	}
	candidates := make([]candidate, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		id := strings.TrimSuffix(name, ".json")
		if !ValidRequestID(id) {
			continue
		}
		info, err := os.Lstat(filepath.Join(s.dir, name))
		if err != nil {
			return nil, 0, fmt.Errorf("inspect receipt %s: %w", id, err)
		}
		candidates = append(candidates, candidate{id: id, updated: info.ModTime()})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].updated.Equal(candidates[j].updated) {
			return candidates[i].id > candidates[j].id
		}
		return candidates[i].updated.After(candidates[j].updated)
	})
	total := len(candidates)
	if limit <= 0 || limit > maxReceiptList {
		limit = maxReceiptList
	}
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	receipts := make([]Receipt, 0, len(candidates))
	for _, candidate := range candidates {
		r, err := s.Get(candidate.id)
		if err != nil {
			return nil, total, fmt.Errorf("load receipt %s: %w", candidate.id, err)
		}
		receipts = append(receipts, r)
	}
	sort.Slice(receipts, func(i, j int) bool { return receipts[i].UpdatedAt > receipts[j].UpdatedAt })
	return receipts, total, nil
}

func (s *Store) ensureDir() error {
	if s == nil || s.dir == "" || !filepath.IsAbs(s.dir) {
		return errors.New("receipt directory must be an absolute path")
	}
	if err := rejectExistingSymlinkComponents(filepath.Dir(s.dir)); err != nil {
		return err
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("create receipt directory: %w", err)
	}
	info, err := os.Lstat(s.dir)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("receipt directory must be a real directory, not a symlink")
	}
	if err := checkReceiptOwner(info); err != nil {
		return fmt.Errorf("receipt directory ownership: %w", err)
	}
	if runtime.GOOS != "windows" {
		if info.Mode().Perm()&0o077 != 0 {
			if err := os.Chmod(s.dir, 0o700); err != nil { // #nosec G302 -- directories need owner search/execute bits; 0700 denies group/other access.
				return fmt.Errorf("secure receipt directory: %w", err)
			}
		}
	}
	return nil
}

func rejectSymlink(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect receipt path: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("refusing symlink receipt path")
	}
	if !info.Mode().IsRegular() {
		return errors.New("receipt path is not a regular file")
	}
	return nil
}

func rejectExistingSymlinkComponents(path string) error {
	volume := filepath.VolumeName(path)
	current := volume + string(os.PathSeparator)
	rest := strings.TrimPrefix(path, current)
	for _, component := range strings.Split(rest, string(os.PathSeparator)) {
		if component == "" {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect receipt directory component: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("receipt directory path contains a symlink")
		}
	}
	return nil
}
