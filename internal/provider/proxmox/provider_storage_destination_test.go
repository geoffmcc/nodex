package proxmox

import (
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/provider/proxmox/client"
	"github.com/geoffmcc/nodex/internal/transport/httpclient"
)

// connectForStorageTest wires a Provider to a TLS test server.
func connectForStorageTest(t *testing.T, handler http.HandlerFunc) *Provider {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)

	certPath := filepath.Join(t.TempDir(), "ca.pem")
	cert := server.Certificate()
	if cert == nil {
		t.Fatal("test server certificate is nil")
	}
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}), 0o600); err != nil {
		t.Fatalf("write CA cert: %v", err)
	}
	caOpt, err := httpclient.WithCACert(certPath)
	if err != nil {
		t.Fatalf("WithCACert: %v", err)
	}
	p := &Provider{}
	creds := &domain.Credentials{Type: "token", TokenID: "root@pam!test", TokenSecret: "secret"}
	if err := p.ConnectWithOptions(server.URL, creds, caOpt); err != nil {
		t.Fatalf("ConnectWithOptions: %v", err)
	}
	return p
}

const clusterResourcesBody = `{"data":[
 {"id":"storage/proxmox/pbs","type":"storage","storage":"pbs","node":"proxmox","status":"available","plugintype":"pbs","maxdisk":956729987072,"disk":224591650816,"content":"backup"},
 {"id":"storage/proxmox/pbs-nightly","type":"storage","storage":"pbs-nightly","node":"proxmox","status":"available","plugintype":"pbs","maxdisk":956729987072,"disk":224591650816,"content":"backup"},
 {"id":"storage/proxmox/local-lvm","type":"storage","storage":"local-lvm","node":"proxmox","status":"available","plugintype":"lvmthin","maxdisk":875485462528,"disk":293287629946,"content":"images,rootdir"}
]}`

// The evaluation reported that three PBS storages returned identical capacity and
// no server, datastore, or path, so an agent could not tell whether a backup
// would land in one datastore or three. Type and destination must both be
// reported, and two names resolving to the same destination must be visibly
// identical rather than indistinguishable.
func TestStorageReportsBackupDestination(t *testing.T) {
	p := connectForStorageTest(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/cluster/resources":
			_, _ = fmt.Fprint(w, clusterResourcesBody)
		case "/api2/json/storage/pbs/config":
			_, _ = fmt.Fprint(w, `{"data":{"storage":"pbs","type":"pbs","server":"pbs-a.example.invalid","datastore":"main","content":"backup"}}`)
		case "/api2/json/storage/pbs-nightly/config":
			// Same destination as "pbs": this is the case the report could not
			// distinguish from a separate datastore.
			_, _ = fmt.Fprint(w, `{"data":{"storage":"pbs-nightly","type":"pbs","server":"pbs-a.example.invalid","datastore":"main","content":"backup"}}`)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	})

	storages, err := p.Storage(t.Context())
	if err != nil {
		t.Fatalf("Storage: %v", err)
	}
	if len(storages) != 3 {
		t.Fatalf("got %d storages, want 3", len(storages))
	}

	byName := make(map[string]domain.Storage, len(storages))
	for _, s := range storages {
		byName[s.Name] = s
	}

	pbs := byName["pbs"]
	if pbs.Type != "pbs" {
		t.Errorf("pbs type = %q, want %q; the row's literal \"storage\" type reports nothing about the backend", pbs.Type, "pbs")
	}
	if pbs.Destination == nil {
		t.Fatalf("pbs has no destination; an agent cannot tell where a backup would land")
	}
	if !pbs.Destination.Resolved {
		t.Errorf("pbs destination resolved = false, reason %q", pbs.Destination.Reason)
	}
	if pbs.Destination.Server != "pbs-a.example.invalid" || pbs.Destination.Datastore != "main" {
		t.Errorf("pbs destination = %+v, want server pbs-a.example.invalid datastore main", pbs.Destination)
	}

	nightly := byName["pbs-nightly"]
	if nightly.Destination == nil || nightly.Destination.Server != pbs.Destination.Server || nightly.Destination.Datastore != pbs.Destination.Datastore {
		t.Errorf("pbs-nightly destination = %+v; two names for one datastore must be visibly identical", nightly.Destination)
	}

	// A non-backup storage has nothing to resolve, and must not gain noise.
	if lvm := byName["local-lvm"]; lvm.Destination != nil {
		t.Errorf("local-lvm destination = %+v, want none; it cannot hold backups", lvm.Destination)
	}
	if lvm := byName["local-lvm"]; lvm.Type != "lvmthin" {
		t.Errorf("local-lvm type = %q, want %q", lvm.Type, "lvmthin")
	}
}

// A read-only audit token may be permitted on /cluster/resources but not on
// /storage/{id}/config. That must not turn a working storage listing into a
// failing one, and the failure must be visible as an unresolved destination
// rather than silently omitted.
func TestStorageDestinationFailureDoesNotFailListing(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     int
		wantReason string
	}{
		{"forbidden", http.StatusForbidden, "forbidden"},
		{"unavailable", http.StatusInternalServerError, "unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := connectForStorageTest(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api2/json/cluster/resources":
					_, _ = fmt.Fprint(w, clusterResourcesBody)
				case "/api2/json/storage/pbs/config", "/api2/json/storage/pbs-nightly/config":
					w.WriteHeader(tc.status)
					_, _ = fmt.Fprint(w, `{"errors":"permission check failed"}`)
				default:
					t.Errorf("unexpected path: %s", r.URL.Path)
					http.NotFound(w, r)
				}
			})

			storages, err := p.Storage(t.Context())
			if err != nil {
				t.Fatalf("Storage returned %v; a destination lookup failure must not fail the listing", err)
			}
			if len(storages) != 3 {
				t.Fatalf("got %d storages, want 3", len(storages))
			}
			for _, s := range storages {
				if s.Name != "pbs" && s.Name != "pbs-nightly" {
					continue
				}
				if s.Destination == nil {
					t.Fatalf("%s has no destination field; absence is ambiguous between \"not applicable\" and \"could not determine\"", s.Name)
				}
				if s.Destination.Resolved {
					t.Errorf("%s resolved = true despite a %d response", s.Name, tc.status)
				}
				if s.Destination.Reason != tc.wantReason {
					t.Errorf("%s reason = %q, want %q", s.Name, s.Destination.Reason, tc.wantReason)
				}
				if s.Destination.Server != "" || s.Destination.Datastore != "" {
					t.Errorf("%s reported destination fields %+v despite a failed lookup", s.Name, s.Destination)
				}
			}
		})
	}
}

// A shared flag that Proxmox omits must not be reported as false.
func TestStorageDestinationSharedIsTriState(t *testing.T) {
	p := connectForStorageTest(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/cluster/resources":
			_, _ = fmt.Fprint(w, clusterResourcesBody)
		case "/api2/json/storage/pbs/config":
			_, _ = fmt.Fprint(w, `{"data":{"storage":"pbs","type":"pbs","server":"pbs-a","datastore":"main","shared":1}}`)
		case "/api2/json/storage/pbs-nightly/config":
			_, _ = fmt.Fprint(w, `{"data":{"storage":"pbs-nightly","type":"pbs","server":"pbs-a","datastore":"main"}}`)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	})

	storages, err := p.Storage(t.Context())
	if err != nil {
		t.Fatalf("Storage: %v", err)
	}
	byName := make(map[string]domain.Storage, len(storages))
	for _, s := range storages {
		byName[s.Name] = s
	}
	if destination := byName["pbs"].Destination; destination == nil {
		t.Fatal("pbs destination is missing")
	} else if shared := destination.Shared; shared == nil || !*shared {
		t.Errorf("pbs shared = %v, want true", shared)
	}
	if destination := byName["pbs-nightly"].Destination; destination == nil {
		t.Fatal("pbs-nightly destination is missing")
	} else if shared := destination.Shared; shared != nil {
		t.Errorf("pbs-nightly shared = %v, want nil; an omitted flag is not a negative one", *shared)
	}
}

// A resolved destination that names no location is not a resolution.
func TestStorageDestinationWithoutAnyFieldIsUnresolved(t *testing.T) {
	p := connectForStorageTest(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/cluster/resources":
			_, _ = fmt.Fprint(w, clusterResourcesBody)
		case "/api2/json/storage/pbs/config", "/api2/json/storage/pbs-nightly/config":
			_, _ = fmt.Fprint(w, `{"data":{"storage":"pbs","type":"pbs"}}`)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	})

	storages, err := p.Storage(t.Context())
	if err != nil {
		t.Fatalf("Storage: %v", err)
	}
	for _, s := range storages {
		if !backupCapable(s.Content) {
			continue
		}
		if s.Destination == nil {
			t.Fatalf("%s has no destination; backup-capable storage must report resolution state", s.Name)
		}
		if s.Destination.Resolved {
			t.Errorf("%s resolved = true with no server, datastore, or path", s.Name)
		}
		if s.Destination.Reason != "no_destination_fields" {
			t.Errorf("%s reason = %q, want %q", s.Name, s.Destination.Reason, "no_destination_fields")
		}
	}
}

func TestGetStorageConfigUsesClusterScopedEndpoint(t *testing.T) {
	p := connectForStorageTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api2/json/storage/pbs/config" {
			t.Errorf("unexpected path: %s; the config endpoint is cluster-scoped", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprint(w, `{"data":{"storage":"pbs","type":"pbs","server":"pbs-a","datastore":"main","path":"/backup","shared":0}}`)
	})

	cfg, err := p.client.GetStorageConfig(t.Context(), "pbs")
	if err != nil {
		t.Fatalf("GetStorageConfig: %v", err)
	}
	if cfg.Server != "pbs-a" || cfg.Datastore != "main" || cfg.Path != "/backup" {
		t.Errorf("config = %+v", cfg)
	}
	if cfg.Shared == nil || *cfg.Shared != 0 {
		t.Errorf("config shared = %v, want 0", cfg.Shared)
	}
	if _, err := p.client.GetStorageConfig(t.Context(), ""); err == nil {
		t.Error("GetStorageConfig with an empty name returned no error")
	}
}

// MapStorage must prefer plugintype and fall back to the row type when absent.
func TestMapStorageTypePrefersPluginType(t *testing.T) {
	withPlugin := MapStorage(client.ClusterResource{Type: "storage", Plugintype: "pbs", Storage: "pbs"})
	if withPlugin.Type != "pbs" {
		t.Errorf("type = %q, want %q", withPlugin.Type, "pbs")
	}
	withoutPlugin := MapStorage(client.ClusterResource{Type: "storage", Storage: "legacy"})
	if withoutPlugin.Type != "storage" {
		t.Errorf("type = %q, want the row type as a fallback", withoutPlugin.Type)
	}
}
