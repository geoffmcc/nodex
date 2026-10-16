package cli

import (
	"context"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/provider/proxmox"
	"github.com/geoffmcc/nodex/internal/transport/httpclient"
)

func TestStatusWatchHTTPSReadOnlyRecovery(t *testing.T) {
	var unavailable atomic.Bool
	var requests, mutations atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodGet {
			mutations.Add(1)
			http.Error(w, "mutation rejected", http.StatusMethodNotAllowed)
			return
		}
		if unavailable.Load() {
			http.Error(w, "fixture outage", http.StatusForbidden)
			return
		}
		var data string
		switch r.URL.Path {
		case "/api2/json/nodes":
			data = `[{"node":"node","status":"online","ip":"192.0.2.1"}]`
		case "/api2/json/version":
			data = `{"version":"9.2.2","release":"9.2"}`
		case "/api2/json/cluster/status":
			data = `[{"type":"node","name":"node","online":1,"local":1}]`
		case "/api2/json/cluster/ha/status":
			data = `{"status":"unknown","quorum":0}`
		case "/api2/json/cluster/resources":
			data = `[{"id":"storage/node/pbs","type":"storage","storage":"pbs","node":"node","plugintype":"pbs","status":"available","maxdisk":100,"disk":25},{"id":"storage/node/nightly","type":"storage","storage":"nightly","node":"node","plugintype":"pbs","status":"available","maxdisk":100,"disk":25}]`
		case "/api2/json/storage":
			data = `[{"storage":"pbs","type":"pbs","server":"backup.example.test","datastore":"store"},{"storage":"nightly","type":"pbs","server":"backup.example.test","datastore":"store","namespace":"nightly"}]`
		case "/api2/json/nodes/node/tasks":
			data = `[{"node":"node","type":"vzdump","status":"running","starttime":1}]`
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprintf(w, `{"data":%s}`, data)
	}))
	defer server.Close()
	caPath := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	ca, err := httpclient.WithCACert(caPath)
	if err != nil {
		t.Fatal(err)
	}
	p := &proxmox.Provider{}
	if err := p.ConnectWithOptions(server.URL, &domain.Credentials{Type: "token", TokenID: "fixture@pam!watch", TokenSecret: "fixture"}, ca); err != nil {
		t.Fatal(err)
	}
	terminal := &fakeWatchTerminal{keys: make(chan byte, 1)}
	completed := 0
	terminal.onDraw = func(frame string) {
		if !strings.Contains(frame, "refreshing") {
			completed++
			if completed == 3 {
				terminal.keys <- 'q'
			}
		}
	}
	cycles := 0
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	err = watchStatus(ctx, terminal, "fixture", time.Millisecond, time.Second, func(ctx context.Context) *statusOverview {
		cycles++
		unavailable.Store(cycles == 2)
		s := collectStatusSnapshot(ctx, p, true)
		if cycles != 2 {
			if !statusRefreshComplete(s) || len(s.Storage) != 2 || len(s.StorageCapacity.Groups) != 1 || len(s.RecentTasks) != 1 {
				t.Errorf("incomplete or inconsistent recovered snapshot: %+v", s)
			}
		}
		return s
	})
	if err != nil || cycles != 3 || mutations.Load() != 0 || requests.Load() == 0 {
		t.Fatalf("watch lifecycle: cycles=%d requests=%d mutations=%d err=%v", cycles, requests.Load(), mutations.Load(), err)
	}
	if len(terminal.frames) != 6 || !strings.Contains(terminal.frames[3], "UNREACHABLE / STALE") || !strings.Contains(terminal.frames[5], "namespace: nightly") || strings.Contains(terminal.frames[5], "STALE") {
		t.Fatalf("outage/recovery did not retain grouped identities: %v", terminal.frames)
	}
}
