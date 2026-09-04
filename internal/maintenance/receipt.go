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
)

const ReceiptSchemaVersion = 1

type HostReceipt struct {
	Host        string `json:"host" yaml:"host"`
	Operation   string `json:"operation" yaml:"operation"`
	State       string `json:"state" yaml:"state"`
	Success     bool   `json:"success" yaml:"success"`
	Changed     int    `json:"changed" yaml:"changed"`
	Failures    int    `json:"failures" yaml:"failures"`
	Unreachable int    `json:"unreachable" yaml:"unreachable"`
}

// Receipt is deliberately an outcome summary. It excludes command output,
// inventory credentials, and provider paths.
type Receipt struct {
	Schema     int           `json:"schema" yaml:"schema"`
	ReceiptID  string        `json:"receipt_id" yaml:"receipt_id"`
	PlanID     string        `json:"plan_id" yaml:"plan_id"`
	PlanDigest string        `json:"plan_digest" yaml:"plan_digest"`
	State      string        `json:"state" yaml:"state"`
	StartedAt  int64         `json:"started_at" yaml:"started_at"`
	UpdatedAt  int64         `json:"updated_at" yaml:"updated_at"`
	Hosts      []HostReceipt `json:"hosts" yaml:"hosts"`
	Error      string        `json:"error,omitempty" yaml:"error,omitempty"`
	Digest     string        `json:"digest" yaml:"digest"`
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
	sort.Slice(r.Hosts, func(i, j int) bool { return r.Hosts[i].Host < r.Hosts[j].Host })
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
	if r.State != "running" && r.State != "succeeded" && r.State != "failed" && r.State != "unknown" {
		return fmt.Errorf("invalid receipt state %q", r.State)
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

func SaveReceipt(path string, r Receipt) error {
	if err := r.Verify(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return atomicwrite.WriteFile(path, append(b, '\n'), true, 0o700, 0o600)
}

func LoadReceipt(path string) (Receipt, error) {
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

func ReceiptPath(dir, planID string) string { return filepath.Join(dir, planID+".receipt.json") }
