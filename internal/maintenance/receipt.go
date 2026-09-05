package maintenance

import (
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

const ReceiptSchemaVersion = 1

type HostReceipt struct {
	Host           string   `json:"host" yaml:"host"`
	Operation      string   `json:"operation" yaml:"operation"`
	State          string   `json:"state" yaml:"state"`
	Success        bool     `json:"success" yaml:"success"`
	Changed        int      `json:"changed" yaml:"changed"`
	Failures       int      `json:"failures" yaml:"failures"`
	Unreachable    int      `json:"unreachable" yaml:"unreachable"`
	Verification   string   `json:"verification" yaml:"verification"`
	Warnings       []string `json:"warnings,omitempty" yaml:"warnings,omitempty"`
	FailuresDetail []string `json:"failures_detail,omitempty" yaml:"failures_detail,omitempty"`
}

type ReceiptEvent struct {
	Host   string `json:"host" yaml:"host"`
	State  string `json:"state" yaml:"state"`
	At     int64  `json:"at" yaml:"at"`
	Detail string `json:"detail,omitempty" yaml:"detail,omitempty"`
}

// Receipt is deliberately an outcome summary. It excludes command output,
// inventory credentials, and provider paths.
type Receipt struct {
	Schema     int            `json:"schema" yaml:"schema"`
	ReceiptID  string         `json:"receipt_id" yaml:"receipt_id"`
	PlanID     string         `json:"plan_id" yaml:"plan_id"`
	PlanDigest string         `json:"plan_digest" yaml:"plan_digest"`
	State      string         `json:"state" yaml:"state"`
	StartedAt  int64          `json:"started_at" yaml:"started_at"`
	UpdatedAt  int64          `json:"updated_at" yaml:"updated_at"`
	Hosts      []HostReceipt  `json:"hosts" yaml:"hosts"`
	Events     []ReceiptEvent `json:"events,omitempty" yaml:"events,omitempty"`
	Error      string         `json:"error,omitempty" yaml:"error,omitempty"`
	Digest     string         `json:"digest" yaml:"digest"`
}

func (r Receipt) digest() (string, error) {
	r.Digest = ""
	b, err := json.Marshal(r)
	if err != nil {
		return "", err
	}
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:]), nil
}

func (r *Receipt) Finalize() error {
	if r.Schema != ReceiptSchemaVersion || r.ReceiptID == "" || r.PlanID == "" || r.PlanDigest == "" {
		return fmt.Errorf("invalid receipt identity")
	}
	if r.StartedAt <= 0 || r.UpdatedAt < r.StartedAt || r.State == "" {
		return fmt.Errorf("invalid receipt timestamps or state")
	}
	sort.Slice(r.Hosts, func(i, j int) bool { return r.Hosts[i].Host < r.Hosts[j].Host })
	sort.Slice(r.Events, func(i, j int) bool {
		if r.Events[i].At != r.Events[j].At {
			return r.Events[i].At < r.Events[j].At
		}
		return r.Events[i].Host < r.Events[j].Host
	})
	if err := validateReceipt(r); err != nil {
		return err
	}
	r.Error = redact.String(r.Error)
	d, err := r.digest()
	if err != nil {
		return err
	}
	r.Digest = d
	return nil
}

func (r Receipt) Verify() error {
	if r.Schema != ReceiptSchemaVersion || r.ReceiptID == "" || r.PlanID == "" || r.PlanDigest == "" {
		return fmt.Errorf("invalid receipt")
	}
	if r.State != "running" && r.State != "succeeded" && r.State != "failed" && r.State != "unknown" && r.State != "cancelled" && r.State != "abandoned" && r.State != "blocked" {
		return fmt.Errorf("invalid receipt state %q", r.State)
	}
	if r.StartedAt <= 0 || r.UpdatedAt < r.StartedAt {
		return fmt.Errorf("invalid receipt timestamps")
	}
	if err := validateReceipt(&r); err != nil {
		return err
	}
	want, err := r.digest()
	if err != nil {
		return err
	}
	if want != r.Digest {
		return fmt.Errorf("receipt digest mismatch: receipt may be tampered or incomplete")
	}
	return nil
}

func validateReceipt(r *Receipt) error {
	seen := make(map[string]bool, len(r.Hosts))
	for _, h := range r.Hosts {
		if h.Host == "" || h.Operation == "" || seen[h.Host] {
			return fmt.Errorf("invalid or duplicate receipt host %q", h.Host)
		}
		if h.State != "scheduled" && h.State != "running" && h.State != "succeeded" && h.State != "failed" && h.State != "unknown" && h.State != "skipped" && h.State != "blocked" && h.State != "not_started" {
			return fmt.Errorf("invalid host receipt state %q", h.State)
		}
		if h.Verification != "" && h.Verification != "pending" && h.Verification != "succeeded" && h.Verification != "failed" && h.Verification != "unknown" {
			return fmt.Errorf("invalid host verification state %q", h.Verification)
		}
		seen[h.Host] = true
	}
	for _, e := range r.Events {
		if e.At <= 0 || e.State == "" {
			return fmt.Errorf("invalid receipt event")
		}
	}
	return nil
}

// VerifyForPlan binds a receipt to the exact plan identity and digest.
func (r Receipt) VerifyForPlan(p Plan) error {
	if err := r.Verify(); err != nil {
		return err
	}
	if r.PlanID != p.PlanID || r.PlanDigest != p.Digest {
		return fmt.Errorf("receipt does not belong to plan %s", p.PlanID)
	}
	return nil
}

func SaveReceipt(path string, r Receipt) error {
	if err := rejectReceiptSymlink(path); err != nil {
		return err
	}
	if err := r.Verify(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	lock, err := config.Lock(path)
	if err != nil {
		return fmt.Errorf("lock receipt: %w", err)
	}
	defer func() { _ = config.Unlock(lock) }()
	return atomicwrite.WriteFile(path, append(b, '\n'), true, 0o700, 0o600)
}

func LoadReceipt(path string) (Receipt, error) {
	if err := rejectReceiptSymlink(path); err != nil {
		return Receipt{}, err
	}
	lock, err := config.Lock(path)
	if err != nil {
		return Receipt{}, fmt.Errorf("lock receipt: %w", err)
	}
	defer func() { _ = config.Unlock(lock) }()
	b, err := os.ReadFile(path) // #nosec G304 -- receipt path is explicitly selected by the operator.
	if err != nil {
		return Receipt{}, fmt.Errorf("read receipt: %w", err)
	}
	var r Receipt
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return Receipt{}, fmt.Errorf("decode receipt: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return Receipt{}, fmt.Errorf("receipt contains multiple JSON values")
		}
		return Receipt{}, fmt.Errorf("read receipt: %w", err)
	}
	if err := r.Verify(); err != nil {
		return Receipt{}, err
	}
	return r, nil
}

func NewReceipt(plan Plan, now time.Time) Receipt {
	id, _ := NewPlanID()
	return Receipt{Schema: ReceiptSchemaVersion, ReceiptID: "mr-" + id[3:], PlanID: plan.PlanID, PlanDigest: plan.Digest, State: "running", StartedAt: now.Unix(), UpdatedAt: now.Unix(), Hosts: []HostReceipt{}}
}

func rejectReceiptSymlink(path string) error {
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing symlink receipt path")
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("receipt path is not a regular file")
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect receipt path: %w", err)
	}
	return nil
}

func ReceiptPath(dir, planID string) string { return filepath.Join(dir, planID+".receipt.json") }
