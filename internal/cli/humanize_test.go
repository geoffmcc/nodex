package cli

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/geoffmcc/nodex/internal/domain"
)

func TestDecorateNodeUptimeUsesExplicitUnits(t *testing.T) {
	uptime := 217 * time.Hour
	node := decorateNode(domain.Node{Uptime: &uptime})

	if node.UptimeNanos != int64(uptime) || node.UptimeSeconds != int64(uptime/time.Second) {
		t.Fatalf("decorated uptime = nanos %d, seconds %d", node.UptimeNanos, node.UptimeSeconds)
	}
	if node.UptimeHuman != "217h0m0s" {
		t.Fatalf("uptime_human = %q, want 217h0m0s", node.UptimeHuman)
	}

	encoded, err := json.Marshal(node)
	if err != nil {
		t.Fatalf("marshal node: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("unmarshal node: %v", err)
	}
	if _, exists := fields["uptime"]; exists {
		t.Fatalf("ambiguous uptime field remains: %s", encoded)
	}
	if got := fields["uptime_nanos"]; got != float64(int64(uptime)) {
		t.Fatalf("uptime_nanos = %v, want %d", got, int64(uptime))
	}
}
