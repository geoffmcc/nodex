package certification

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/task"
)

const RequiredProfile = "nodex-test-admin"

// Authorization is the exact environment binding required for mutation
// suites. It is intentionally independent of profile names and resource
// prefixes.
type Authorization struct {
	Environment         string
	Profile             string
	Endpoint            string
	Provider            string
	ExpectedFingerprint string
	TrustedCAIdentity   string
	Nodes               []string
	Storage             []string
	VMIDMin             int
	VMIDMax             int
	Suites              []string
	AllowMutations      bool
	MaxResources        int
	ExpiresAt           int64
}

type Request struct {
	Profile, Node, Name, Storage string
	VMID                         int
	ConfirmTarget                string
	OptIn                        bool
	Environment                  string
	Suite                        string
	Endpoint                     string
	CAFile                       string
	Authorization                *Authorization
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

func ValidateAuthorization(r Request) error {
	a := r.Authorization
	if a == nil {
		return fmt.Errorf("certification requires an explicit authorization binding")
	}
	if a.Environment == "" || a.Profile != RequiredProfile || r.Environment != a.Environment || r.Profile != a.Profile || r.Endpoint != a.Endpoint {
		return fmt.Errorf("certification target does not match the authorized environment")
	}
	if a.Provider == "" || a.ExpectedFingerprint == "" || a.TrustedCAIdentity == "" || a.VMIDMin <= 0 || r.VMID < a.VMIDMin || r.VMID > a.VMIDMax {
		return fmt.Errorf("certification target is outside the authorized resource range")
	}
	if r.Suite == "disposable-mutations" && !a.AllowMutations {
		return fmt.Errorf("mutation suite is not authorized")
	}
	if !contains(a.Suites, r.Suite) {
		return fmt.Errorf("suite %q is not authorized", r.Suite)
	}
	if a.ExpiresAt <= time.Now().Unix() {
		return fmt.Errorf("certification authorization is expired")
	}
	if !contains(a.Nodes, r.Node) || !contains(a.Storage, r.Storage) {
		return fmt.Errorf("certification node or storage is not authorized")
	}
	return nil
}
func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// VerifyEndpointIdentity performs the TLS identity check before any provider
// mutation. The authorization stores the leaf certificate fingerprint so a
// DNS or endpoint substitution cannot silently reach a different environment.
func VerifyEndpointIdentity(ctx context.Context, endpoint, caFile, expected, expectedCA string) error {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return fmt.Errorf("certification endpoint is not a valid HTTPS URL")
	}
	roots, err := x509.SystemCertPool()
	if err != nil {
		return fmt.Errorf("load system trust store")
	}
	if caFile != "" {
		roots = x509.NewCertPool()
		pem, readErr := os.ReadFile(caFile) // #nosec G304 -- CA path comes from the authorized profile.
		if readErr != nil || !roots.AppendCertsFromPEM(pem) {
			return fmt.Errorf("load configured trust anchor")
		}
	}
	conn, err := (&tls.Dialer{NetDialer: &net.Dialer{}, Config: &tls.Config{MinVersion: tls.VersionTLS12, ServerName: u.Hostname(), RootCAs: roots}}).DialContext(ctx, "tcp", u.Host)
	if err != nil {
		return fmt.Errorf("verify certification endpoint TLS identity")
	}
	defer conn.Close()
	tlsConn, ok := conn.(*tls.Conn)
	if !ok || len(tlsConn.ConnectionState().PeerCertificates) == 0 {
		return fmt.Errorf("certification endpoint returned no certificate")
	}
	sum := sha256.Sum256(tlsConn.ConnectionState().PeerCertificates[0].Raw)
	if !strings.EqualFold(hex.EncodeToString(sum[:]), expected) {
		return fmt.Errorf("certification endpoint fingerprint does not match authorization")
	}
	chains := tlsConn.ConnectionState().VerifiedChains
	if len(chains) == 0 || len(chains[0]) == 0 {
		return fmt.Errorf("certification endpoint trust chain is unavailable")
	}
	caSum := sha256.Sum256(chains[0][len(chains[0])-1].Raw)
	if !strings.EqualFold(hex.EncodeToString(caSum[:]), expectedCA) {
		return fmt.Errorf("certification trusted CA identity does not match authorization")
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
	if err := ValidateAuthorization(req); err != nil {
		return Result{}, err
	}
	if !strings.EqualFold(p.Name(), req.Authorization.Provider) {
		return Result{}, fmt.Errorf("connected provider does not match certification authorization")
	}
	if err := VerifyEndpointIdentity(ctx, req.Endpoint, req.CAFile, req.Authorization.ExpectedFingerprint, req.Authorization.TrustedCAIdentity); err != nil {
		return Result{}, err
	}
	if req.Suite == "readonly" {
		if err := p.Health(ctx); err != nil {
			return Result{}, fmt.Errorf("certification read-only health check: %w", err)
		}
		return Result{Profile: req.Profile, Target: req.Name, State: "succeeded", Cleanup: "not_applicable", Ledger: ""}, nil
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
	e := NewEntry(req.Profile, req.Node, req.VMID, req.Name, req.Storage, now)
	e.Environment, e.EndpointIdentity, e.ProviderIdentity = req.Environment, req.Endpoint, req.Authorization.Provider
	e.Fingerprint, e.CAIdentity, e.RunID = req.Authorization.ExpectedFingerprint, req.Authorization.TrustedCAIdentity, e.ID
	l, err := Reserve(ledgerPath, e, req.Authorization.MaxResources)
	if err != nil {
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
	if err := UpdateEntry(ledgerPath, e.ID, func(entry *Entry) { entry.State, entry.UpdatedAt = "creating", time.Now().Unix() }); err != nil {
		return result, fmt.Errorf("checkpoint certification creation: %w", err)
	}
	upid, err := creator.VMCreate(ctx, req.Node, req.VMID, req.Name, "", req.Storage)
	if err != nil {
		return result, fmt.Errorf("create certification VM: %w", err)
	}
	if err := waitTask(ctx, p, req.Node, upid); err != nil {
		return result, fmt.Errorf("create certification VM: %w", err)
	}
	if err := UpdateEntry(ledgerPath, e.ID, func(entry *Entry) { entry.State, entry.UpdatedAt = "created", time.Now().Unix() }); err != nil {
		return result, fmt.Errorf("checkpoint created certification VM: %w", err)
	}
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
	if err := UpdateEntry(ledgerPath, e.ID, func(entry *Entry) {
		entry.State, entry.Cleanup, entry.UpdatedAt = "succeeded", "complete", time.Now().Unix()
	}); err != nil {
		return result, err
	}
	result.State, result.Cleanup = "succeeded", "complete"
	return result, nil
}

func Cleanup(ctx context.Context, p domain.Provider, ledgerPath, confirm string, auth *Authorization, caFile string) ([]Result, error) {
	if confirm == "" {
		return nil, fmt.Errorf("cleanup requires exact --confirm-target <ledger-entry-id>")
	}
	if auth == nil {
		return nil, fmt.Errorf("cleanup requires an explicit authorization binding")
	}
	if !strings.EqualFold(p.Name(), auth.Provider) {
		return nil, fmt.Errorf("connected provider does not match cleanup authorization")
	}
	if err := VerifyEndpointIdentity(ctx, auth.Endpoint, caFile, auth.ExpectedFingerprint, auth.TrustedCAIdentity); err != nil {
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
	e, err := ClaimCleanup(ledgerPath, confirm, auth)
	if err != nil {
		return nil, err
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
		if err := UpdateEntry(ledgerPath, e.ID, func(entry *Entry) {
			entry.State, entry.Cleanup, entry.UpdatedAt = "cleaned", "complete", time.Now().Unix()
		}); err != nil {
			return nil, err
		}
		return []Result{{Profile: e.Profile, Target: e.Name, State: "cleaned", Cleanup: "complete", Ledger: ledgerPath}}, nil
	}
	upid, err := d.VMDelete(ctx, e.Node, e.VMID)
	if err != nil {
		if saveErr := UpdateEntry(ledgerPath, e.ID, func(entry *Entry) {
			entry.State, entry.Error, entry.UpdatedAt = "unknown", "cleanup request failed", time.Now().Unix()
		}); saveErr != nil {
			return nil, fmt.Errorf("persist cleanup failure: %w", saveErr)
		}
		return nil, err
	}
	if err := waitTask(ctx, p, e.Node, upid); err != nil {
		if saveErr := UpdateEntry(ledgerPath, e.ID, func(entry *Entry) {
			entry.State, entry.Error, entry.UpdatedAt = "unknown", "cleanup task outcome unavailable", time.Now().Unix()
		}); saveErr != nil {
			return nil, fmt.Errorf("persist cleanup failure: %w", saveErr)
		}
		return nil, err
	}
	vms, err = inspector.VMs(ctx)
	if err != nil {
		return nil, err
	}
	for _, vm := range vms {
		if vm.ID == targetID {
			if saveErr := UpdateEntry(ledgerPath, e.ID, func(entry *Entry) {
				entry.State, entry.Error, entry.UpdatedAt = "unknown", "target still exists", time.Now().Unix()
			}); saveErr != nil {
				return nil, fmt.Errorf("persist cleanup ambiguity: %w", saveErr)
			}
			return nil, fmt.Errorf("cleanup target still exists")
		}
	}
	if err := UpdateEntry(ledgerPath, e.ID, func(entry *Entry) {
		entry.State, entry.Cleanup, entry.UpdatedAt = "cleaned", "complete", time.Now().Unix()
	}); err != nil {
		return nil, err
	}
	return []Result{{Profile: e.Profile, Target: e.Name, State: "cleaned", Cleanup: "complete", Ledger: ledgerPath}}, nil
}
