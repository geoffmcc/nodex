// Command generate-mutation-safety-matrix writes the reviewable mutation
// inventory from the same registry used by command routing and agent discovery.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/geoffmcc/nodex/internal/cli"
)

func main() {
	var encoded bytes.Buffer
	if err := cli.Run(context.Background(), []string{"--output", "json", "operation", "list"}, &encoded, io.Discard); err != nil {
		fail(err)
	}
	var operations []cli.OperationSummary
	if err := json.Unmarshal(encoded.Bytes(), &operations); err != nil {
		fail(err)
	}
	var doc strings.Builder
	doc.WriteString(strings.Join([]string{
		"# Mutation safety and recovery inventory",
		"",
		"This table is generated from the canonical command registry and the same",
		"operation summaries exposed by `nodex operation list`. Regenerate after",
		"changing operation metadata with `go run ./scripts/generate-mutation-safety-matrix.go`.",
		"",
		"PVE/PBS HTTP mutations use the client's single-attempt `DoMutation` path.",
		"SSH/Ansible mutations use one remote invocation. Neither path automatically",
		"retries a mutation; a transport error or server error can leave its result",
		"ambiguous. Agent mode writes an unknown receipt before invoking a mutation.",
		"Repeating its request ID with identical normalized input returns that receipt",
		"without resubmission; different input or target is rejected. Receipts are",
		"local diagnostic/deduplication state, not authorization. Ordinary mutation",
		"invocations keep their existing confirmation gates and create no agent receipt.",
		"Explicit receipt-management commands may update the local receipt ledger.",
		"",
		"Task completion and postcondition verification are separate. The verification",
		"and recovery columns describe the declared operation contract; a missing",
		"postcondition remains unsupported or unknown where provider evidence is absent.",
		"Registry metadata cannot certify live provider permissions or guarantee",
		"exactly-once execution where a provider exposes no idempotency key.",
		"",
		"| Operation | Tier | Confirmation | Retry / ambiguous outcome | Task / wait | Agent receipt | Verification and recovery |",
		"|---|---|---|---|---|---|---|",
		"",
	}, "\n"))
	for _, operation := range operations {
		meta := cli.LookupOperation(operation.Path)
		if meta == nil || meta.Inspection {
			continue
		}
		task := "no task"
		if meta.ProducesUPID {
			task = "UPID"
			if meta.Waitable {
				task += "; --wait supported"
			}
		}
		receipt := "yes"
		if !operation.AgentSupported {
			receipt = "no; " + operation.AgentUnsupportedWhy
		}
		retry := "no mutation is issued by this operation"
		if contains(operation.RemoteSideEffects, "remote_mutation") {
			retry = "single attempt; ambiguous result: do not retry automatically"
		} else if len(operation.LocalSideEffects) != 0 {
			retry = "local effect; inspect current state before repeating"
		}
		evidence := append(append([]string(nil), operation.Verification...), operation.Recovery...)
		row := []string{
			"`" + operation.Path + "`",
			operation.SafetyTier,
			strings.Join(operation.Confirmation, "; "),
			retry,
			task,
			receipt,
			strings.Join(evidence, "; "),
		}
		for i := range row {
			row[i] = cell(row[i])
		}
		doc.WriteString("| " + strings.Join(row, " | ") + " |\n")
	}
	// #nosec G306 -- generated documentation is intentionally world-readable.
	if err := os.WriteFile("docs/mutation-safety-matrix.md", []byte(doc.String()), 0o644); err != nil {
		fail(err)
	}
}

func cell(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "|", "\\|")
	return strings.Join(strings.Fields(value), " ")
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
