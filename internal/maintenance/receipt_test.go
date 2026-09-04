package maintenance

import (
	"strings"
	"testing"
	"time"
)

func TestLoadRejectsUnknownAndTamperedPlan(t *testing.T) {
	p := Plan{Schema: PlanSchemaVersion, PlanID: "mp-test", CreatedAt: 100, ExpiresAt: 200, Policy: PolicySecurityOnly, Hosts: []PlanHost{{Name: "h", Address: "a", Criticality: "standard"}}, HostOrder: []string{"h"}, BatchSize: 1, RebootPolicy: RebootPolicyNever, SafetyClassification: "disruptive"}
	p, _ = Finalize(p)
	b := `{"schema":1,"plan_id":"mp-test","created_at":100,"expires_at":200,"policy":"security-only","hosts":[{"name":"h","address":"a","criticality":"standard"}],"host_order":["h"],"batch_size":1,"reboot_policy":"never","safety_classification":"disruptive","digest":"` + p.Digest + `","extra":true}`
	if _, err := Load(strings.NewReader(b), "json", time.Unix(150, 0)); err == nil {
		t.Fatal("unknown field accepted")
	}
}

func TestReceiptTamperDetected(t *testing.T) {
	p := Plan{Schema: PlanSchemaVersion, PlanID: "mp-test", CreatedAt: 100, ExpiresAt: 200, Policy: PolicySecurityOnly, Hosts: []PlanHost{{Name: "h", Address: "a", Criticality: "standard"}}, HostOrder: []string{"h"}, BatchSize: 1, RebootPolicy: RebootPolicyNever, SafetyClassification: "disruptive"}
	p, _ = Finalize(p)
	r := NewReceipt(p, time.Unix(120, 0))
	if err := r.Finalize(); err != nil {
		t.Fatal(err)
	}
	r.State = "succeeded"
	if err := r.Verify(); err == nil {
		t.Fatal("tampered receipt accepted")
	}
}
