package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/domain"
)

// TestPluralListsPairWithSingularMutations enforces the #33 command-naming
// convention: every plural noun in a resource group is a read-only list, and
// every singular noun is the mutate verb for that same resource.
func TestPluralListsPairWithSingularMutations(t *testing.T) {
	groups := map[string][]string{
		"sdn":      {"zones", "vnets", "subnets", "controllers"},
		"firewall": {"aliases", "ipsets", "security-groups"},
		"access":   {"users"},
	}
	singulars := map[string][]string{
		"sdn":      {"zone", "vnet", "subnet", "controller"},
		"firewall": {"alias", "ipset", "security-group", "rule"},
		"access":   {"user"},
	}

	for group, plurals := range groups {
		parent, ok := GetCommand(group)
		if !ok {
			t.Fatalf("%s command not registered", group)
		}
		for _, name := range plurals {
			cmd, ok := parent.sub[name]
			if !ok {
				t.Fatalf("%s %s: plural list command not registered", group, name)
			}
			if !strings.HasPrefix(strings.ToLower(cmd.short), "list") {
				t.Errorf("%s %s: plural command must be a list, got short %q", group, name, cmd.short)
			}
			if cmd.meta == nil || !cmd.meta.Inspection {
				t.Errorf("%s %s: plural command must be inspection-only", group, name)
			}
		}
		for _, name := range singulars[group] {
			cmd, ok := parent.sub[name]
			if !ok {
				t.Fatalf("%s %s: singular mutate command not registered", group, name)
			}
			if cmd.meta == nil {
				t.Errorf("%s %s: singular command has no operation metadata", group, name)
			}
		}
	}
}

// TestAccessRolesAndGroupsAreReadOnly pins the documented exception: PVE exposes
// no create/delete API for roles or groups, so their plural commands must not
// advertise management and must have no singular counterpart.
func TestAccessRolesAndGroupsAreReadOnly(t *testing.T) {
	access, ok := GetCommand("access")
	if !ok {
		t.Fatal("access command not registered")
	}
	for _, name := range []string{"roles", "groups"} {
		cmd, ok := access.sub[name]
		if !ok {
			t.Fatalf("access %s not registered", name)
		}
		if !strings.HasPrefix(strings.ToLower(cmd.short), "list") {
			t.Errorf("access %s: must be described as a list, got %q", name, cmd.short)
		}
	}
	// Singular role/group commands would be unimplementable against PVE.
	for _, name := range []string{"role", "group"} {
		if _, ok := access.sub[name]; ok {
			t.Errorf("access %s: PVE has no create/delete API, so this command must not exist", name)
		}
	}
}

// TestFirewallGroupIsLegacyAliasForSecurityGroup verifies the singular security
// group mutate verb is reachable under both the canonical and legacy names.
func TestFirewallGroupIsLegacyAliasForSecurityGroup(t *testing.T) {
	canonical := LookupOperation("firewall security-group")
	if canonical == nil {
		t.Fatal("firewall security-group has no operation metadata")
	}
	legacy := LookupOperation("firewall group")
	if legacy == nil {
		t.Fatal("firewall group has no operation metadata")
	}
	if legacy.HandlerFunc != canonical.HandlerFunc {
		t.Errorf("alias handler mismatch: %q vs %q", legacy.HandlerFunc, canonical.HandlerFunc)
	}
	if !strings.Contains(canonical.Description, "security group") {
		t.Errorf("canonical description should describe security groups, got %q", canonical.Description)
	}
}

// TestFirewallRuleScopesHaveDistinctNames pins nit #34: the three firewall rule
// scopes must be distinguishable by name, and the cluster-wide scope must be
// reachable under the historical `list` and `rules` spellings.
func TestFirewallRuleScopesHaveDistinctNames(t *testing.T) {
	firewall, ok := GetCommand("firewall")
	if !ok {
		t.Fatal("firewall command not registered")
	}
	for _, name := range []string{"cluster-rules", "node-rules", "vm-rules"} {
		cmd, ok := firewall.sub[name]
		if !ok {
			t.Fatalf("firewall %s: scoped rule command not registered", name)
		}
		if cmd.meta == nil || !cmd.meta.Inspection {
			t.Errorf("firewall %s: must be a registered inspection command", name)
		}
		if !strings.Contains(strings.ToLower(cmd.short), "rules") {
			t.Errorf("firewall %s: description should name the rule scope, got %q", name, cmd.short)
		}
	}

	// All three cluster-scope spellings must resolve to the same handler.
	canonical := LookupOperation("firewall cluster-rules")
	if canonical == nil {
		t.Fatal("firewall cluster-rules has no operation metadata")
	}
	if !strings.Contains(canonical.Description, "cluster-wide") {
		t.Errorf("cluster-rules description must state its scope, got %q", canonical.Description)
	}
	for _, alias := range []string{"firewall list", "firewall rules"} {
		op := LookupOperation(alias)
		if op == nil {
			t.Fatalf("%s: alias has no operation metadata", alias)
		}
		if op.HandlerFunc != canonical.HandlerFunc {
			t.Errorf("%s: alias handler mismatch: %q vs %q", alias, op.HandlerFunc, canonical.HandlerFunc)
		}
		if !strings.Contains(op.Description, "cluster-wide") {
			t.Errorf("%s: alias description must state its scope, got %q", alias, op.Description)
		}
	}
}

func TestWriteSDNSubnetsTable(t *testing.T) {
	subnets := []domain.SDNSubnet{
		{Name: "10.0.0.0-10.0.0.255", Type: "vnet", VNet: "vnetA", Zone: "simple", CIDR: "24", Gateway: "10.0.0.1"},
		{Name: "10.0.1.0-10.0.1.255", Type: "vnet", VNet: "vnetB", Zone: "simple", CIDR: "24"},
	}

	t.Run("table", func(t *testing.T) {
		buf := &bytes.Buffer{}
		ctx := &Context{Writer: buf, Opts: Options{Output: "table"}}
		if err := writeSDNSubnetsTable(ctx, subnets); err != nil {
			t.Fatalf("writeSDNSubnetsTable: %v", err)
		}
		out := buf.String()
		for _, want := range []string{"SUBNET", "TYPE", "VNET", "ZONE", "CIDR", "GATEWAY", "vnetA", "10.0.0.1"} {
			if !strings.Contains(out, want) {
				t.Errorf("table output missing %q:\n%s", want, out)
			}
		}
	})

	t.Run("json", func(t *testing.T) {
		buf := &bytes.Buffer{}
		ctx := &Context{Writer: buf, Opts: Options{Output: "json"}}
		if err := writeSDNSubnetsTable(ctx, subnets); err != nil {
			t.Fatalf("writeSDNSubnetsTable json: %v", err)
		}
		out := buf.String()
		for _, want := range []string{`"name": "10.0.0.0-10.0.0.255"`, `"vnet": "vnetA"`, `"gateway": "10.0.0.1"`} {
			if !strings.Contains(out, want) {
				t.Errorf("json output missing %q:\n%s", want, out)
			}
		}
	})

	// A nil slice must render as an empty collection, not null.
	t.Run("nil", func(t *testing.T) {
		buf := &bytes.Buffer{}
		ctx := &Context{Writer: buf, Opts: Options{Output: "json"}}
		if err := writeSDNSubnetsTable(ctx, nil); err != nil {
			t.Fatalf("writeSDNSubnetsTable nil: %v", err)
		}
		if got := strings.TrimSpace(buf.String()); got != "[]" {
			t.Errorf("nil subnets should render as [], got %q", got)
		}
	})
}

func TestWriteSDNControllersTable(t *testing.T) {
	controllers := []domain.SDNController{
		{Name: "evpn1", Type: "evpn", State: "online", ASN: 65000},
		{Name: "bgp1", Type: "bgp"},
	}

	t.Run("table", func(t *testing.T) {
		buf := &bytes.Buffer{}
		ctx := &Context{Writer: buf, Opts: Options{Output: "table"}}
		if err := writeSDNControllersTable(ctx, controllers); err != nil {
			t.Fatalf("writeSDNControllersTable: %v", err)
		}
		out := buf.String()
		for _, want := range []string{"NAME", "TYPE", "STATE", "ASN", "evpn1", "online", "65000"} {
			if !strings.Contains(out, want) {
				t.Errorf("table output missing %q:\n%s", want, out)
			}
		}
		// A controller with ASN 0 renders an empty ASN cell, never "0".
		for _, line := range strings.Split(strings.TrimSpace(out), "\n")[2:] {
			if fields := strings.Fields(line); len(fields) > 0 && fields[0] == "bgp1" {
				for _, f := range fields[1:] {
					if f == "0" {
						t.Errorf("zero ASN should render empty:\n%s", out)
					}
				}
			}
		}
	})

	t.Run("json", func(t *testing.T) {
		buf := &bytes.Buffer{}
		ctx := &Context{Writer: buf, Opts: Options{Output: "json"}}
		if err := writeSDNControllersTable(ctx, controllers); err != nil {
			t.Fatalf("writeSDNControllersTable json: %v", err)
		}
		out := buf.String()
		for _, want := range []string{`"name": "evpn1"`, `"asn": 65000`, `"name": "bgp1"`} {
			if !strings.Contains(out, want) {
				t.Errorf("json output missing %q:\n%s", want, out)
			}
		}
	})

	t.Run("nil", func(t *testing.T) {
		buf := &bytes.Buffer{}
		ctx := &Context{Writer: buf, Opts: Options{Output: "json"}}
		if err := writeSDNControllersTable(ctx, nil); err != nil {
			t.Fatalf("writeSDNControllersTable nil: %v", err)
		}
		if got := strings.TrimSpace(buf.String()); got != "[]" {
			t.Errorf("nil controllers should render as [], got %q", got)
		}
	})
}
