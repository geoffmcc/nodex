package cli

import (
	"os"
	"strings"
	"testing"
)

func TestMutationSafetyMatrixCoversCanonicalRegistry(t *testing.T) {
	contents, err := os.ReadFile("../../docs/mutation-safety-matrix.md")
	if err != nil {
		t.Fatal(err)
	}
	rows := make(map[string][]string)
	for _, line := range strings.Split(string(contents), "\n") {
		if !strings.HasPrefix(line, "| `") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		if len(cells) != 7 {
			t.Errorf("malformed mutation matrix row: %s", line)
			continue
		}
		path := strings.Trim(strings.TrimSpace(cells[0]), " `")
		if _, duplicate := rows[path]; duplicate {
			t.Errorf("duplicate mutation matrix row %q", path)
		}
		rows[path] = cells
	}
	mutations := MutationOperations()
	if len(rows) != len(mutations) {
		t.Fatalf("matrix has %d rows; canonical registry has %d mutations", len(rows), len(mutations))
	}
	for _, operation := range mutations {
		cells, exists := rows[operation.Path]
		if !exists {
			t.Errorf("mutation %q is absent from the safety matrix", operation.Path)
			continue
		}
		if got := strings.TrimSpace(cells[1]); got != operation.SafetyTier.String() {
			t.Errorf("%q matrix tier = %q; registry tier = %q", operation.Path, got, operation.SafetyTier)
		}
		summary := summarizeOperation(operation)
		if got, want := strings.TrimSpace(cells[2]), strings.Join(summary.Confirmation, "; "); got != want {
			t.Errorf("%q matrix confirmation = %q; operation contract = %q", operation.Path, got, want)
		}
		wantRetry := "no mutation is issued by this operation"
		if containsString(summary.RemoteSideEffects, "remote_mutation") {
			wantRetry = "single attempt; ambiguous result: do not retry automatically"
		} else if len(summary.LocalSideEffects) != 0 {
			wantRetry = "local effect; inspect current state before repeating"
		}
		if got := strings.TrimSpace(cells[3]); got != wantRetry {
			t.Errorf("%q matrix retry policy = %q; operation effects require %q", operation.Path, got, wantRetry)
		}
		wantTask := "no task"
		if operation.ProducesUPID {
			wantTask = "UPID"
			if operation.Waitable {
				wantTask += "; --wait supported"
			}
		}
		if got := strings.TrimSpace(cells[4]); got != wantTask {
			t.Errorf("%q matrix task metadata = %q; registry = %q", operation.Path, got, wantTask)
		}
		wantReceipt := "yes"
		if !summary.AgentSupported {
			wantReceipt = "no; " + summary.AgentUnsupportedWhy
		}
		if got := strings.TrimSpace(cells[5]); got != wantReceipt {
			t.Errorf("%q matrix receipt coverage = %q; operation contract = %q", operation.Path, got, wantReceipt)
		}
		wantEvidence := strings.Join(append(append([]string(nil), summary.Verification...), summary.Recovery...), "; ")
		if got := strings.TrimSpace(cells[6]); got != wantEvidence {
			t.Errorf("%q matrix evidence = %q; operation contract = %q", operation.Path, got, wantEvidence)
		}
	}
}
