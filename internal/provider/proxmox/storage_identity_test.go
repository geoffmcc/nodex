package proxmox

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/provider/proxmox/client"
)

func TestPBSCapacityIdentity(t *testing.T) {
	base := client.StorageConfig{Type: "pbs", Server: "pbs.example.invalid", Datastore: "main"}
	want := pbsCapacityIdentity(&base, nil).Key
	if want == "" {
		t.Fatal("complete config has no identity")
	}
	unknown := base
	unknown.Server, unknown.Namespace = "localhost", "nightly"
	if id := pbsCapacityIdentity(&unknown, nil); id.Key != "" || id.Namespace != "nightly" {
		t.Fatal("unresolved server must preserve the known namespace view")
	}
	for _, tc := range []struct {
		name             string
		change           func(*client.StorageConfig)
		same, unresolved bool
	}{
		{"normalized host and default port", func(c *client.StorageConfig) { c.Server = "PBS.EXAMPLE.INVALID."; c.Port = 8007 }, true, false},
		{"namespace is a view", func(c *client.StorageConfig) { c.Namespace = "nightly/team" }, true, false},
		{"different datastore", func(c *client.StorageConfig) { c.Datastore = "other" }, false, false},
		{"datastore case matters", func(c *client.StorageConfig) { c.Datastore = "Main" }, false, false},
		{"endpoint alias unproven", func(c *client.StorageConfig) { c.Server = "alias.example.invalid" }, false, false},
		{"different port", func(c *client.StorageConfig) { c.Port = 9007 }, false, false},
		{"missing datastore", func(c *client.StorageConfig) { c.Datastore = "" }, false, true},
		{"missing server", func(c *client.StorageConfig) { c.Server = "" }, false, true},
		{"userinfo", func(c *client.StorageConfig) { c.Server = "user:password@pbs.example.invalid" }, false, true},
		{"url instead of host", func(c *client.StorageConfig) { c.Server = "https://pbs.example.invalid/path?token=secret" }, false, true},
		{"loopback", func(c *client.StorageConfig) { c.Server = "127.0.0.1" }, false, true},
		{"localhost", func(c *client.StorageConfig) { c.Server = "localhost" }, false, true},
		{"node relative", func(c *client.StorageConfig) { c.Server = "pbs" }, false, true},
		{"link local", func(c *client.StorageConfig) { c.Server = "fe80::1" }, false, true},
		{"port range", func(c *client.StorageConfig) { c.Port = 65536 }, false, true},
		{"malicious namespace", func(c *client.StorageConfig) { c.Namespace = "team\x1b[2J" }, false, true},
		{"conflicting type", func(c *client.StorageConfig) { c.Type = "dir" }, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base
			tc.change(&cfg)
			got := pbsCapacityIdentity(&cfg, nil)
			if (got.Key == want) != tc.same || (got.Key == "") != tc.unresolved {
				t.Fatalf("identity = %+v", got)
			}
			if tc.unresolved && got.Reason == "" {
				t.Fatal("missing uncertainty reason")
			}
		})
	}
	for _, hosts := range [][2]string{{"2001:db8::1", "2001:0db8:0:0:0:0:0:1"}, {"::ffff:192.0.2.4", "192.0.2.4"}} {
		a, b := base, base
		a.Server = hosts[0]
		b.Server = hosts[1]
		if pbsCapacityIdentity(&a, nil).Key != pbsCapacityIdentity(&b, nil).Key {
			t.Fatal("IP spelling changed identity")
		}
	}
}

func TestStorageCapacityEvidenceHTTP(t *testing.T) {
	calls := map[string]int{}
	p := connectForStorageTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("mutation called: %s", r.Method)
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		calls[r.URL.Path]++
		switch r.URL.Path {
		case "/api2/json/cluster/resources":
			fmt.Fprint(w, `{"data":[
			{"type":"storage","storage":"pbs","node":"a","plugintype":"pbs","status":"available","disk":20,"maxdisk":100},
			{"type":"storage","storage":"pbs","node":"b","plugintype":"pbs","status":"available","disk":20,"maxdisk":100},
			{"type":"storage","storage":"nightly","node":"a","plugintype":"pbs","status":"available","disk":20,"maxdisk":100},
			{"type":"storage","storage":"shutdown","node":"a","plugintype":"pbs","status":"available","disk":20,"maxdisk":100},
			{"type":"storage","storage":"denied","node":"a","plugintype":"pbs","status":"unknown"},
			{"type":"storage","storage":"shared","node":"a","plugintype":"nfs","shared":1},
			{"type":"storage","storage":"shared","node":"b","plugintype":"nfs","shared":1},
			{"type":"storage","storage":"local","node":"a","plugintype":"dir"},
			{"type":"storage","storage":"local","node":"b","plugintype":"dir"}]}`)
		case "/api2/json/storage":
			fmt.Fprint(w, `{"data":[
			{"storage":"pbs","type":"pbs","server":"pbs.example.invalid","datastore":"main","username":"user@pbs!private","password":"not-for-output","encryption-key":"not-for-output","disable":0},
			{"storage":"nightly","type":"pbs","server":"pbs.example.invalid","datastore":"main","namespace":"nightly"},
			{"storage":"shutdown","type":"pbs","server":"pbs.example.invalid","datastore":"main"}]}`)
		default:
			t.Errorf("unexpected API path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	})
	rows, err := p.Storage(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 9 {
		t.Fatalf("rows=%d", len(rows))
	}
	for i := 0; i < 4; i++ {
		if rows[i].CapacityIdentity.Key == "" || rows[i].CapacityIdentity.Key != rows[0].CapacityIdentity.Key {
			t.Fatalf("PBS aliases not grouped: %+v", rows[i])
		}
	}
	if rows[2].CapacityIdentity.Namespace != "nightly" {
		t.Fatal("namespace lost")
	}
	if rows[4].CapacityIdentity.Reason != "config_not_visible_or_ambiguous" || rows[4].CapacityIdentity.Key != "" {
		t.Fatal("forbidden identity guessed")
	}
	if !reflect.DeepEqual(rows[5].CapacityIdentity, rows[6].CapacityIdentity) || rows[5].CapacityIdentity.Key == "" {
		t.Fatal("shared node views not identified")
	}
	if rows[7].CapacityIdentity != nil || rows[8].CapacityIdentity != nil {
		t.Fatal("local capacities inferred")
	}
	if calls["/api2/json/storage"] != 1 {
		t.Fatal("config list not fetched exactly once")
	}
	data, _ := json.Marshal(rows)
	for _, secret := range []string{"user@pbs", "not-for-output", "password", "encryption-key"} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("secret field leaked: %s", secret)
		}
	}
}

func TestStorageRejectsAmbiguousConfiguration(t *testing.T) {
	p := connectForStorageTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected mutation %s", r.Method)
		}
		switch r.URL.Path {
		case "/api2/json/cluster/resources":
			fmt.Fprint(w, clusterResourcesBody)
		case "/api2/json/storage":
			// Even equal duplicates cannot establish a unique config record.
			fmt.Fprint(w, `{"data":[{"storage":"pbs","type":"pbs","server":"pbs.example.invalid","datastore":"main"},{"storage":"pbs","type":"pbs","server":"pbs.example.invalid","datastore":"main"}]}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	})
	rows, err := p.Storage(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows[:2] {
		if row.CapacityIdentity.Key != "" || row.CapacityIdentity.Reason != "config_not_visible_or_ambiguous" || row.Destination.Resolved {
			t.Fatalf("guessed identity from duplicate/missing config: %+v", row)
		}
	}
}
