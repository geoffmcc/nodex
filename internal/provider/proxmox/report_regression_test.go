package proxmox

import (
	"errors"
	"fmt"
	"github.com/geoffmcc/nodex/internal/app"
	"net/http"
	"testing"
)

func TestReportBackupTaskNormalization(t *testing.T) {
	p := connectForStorageTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api2/json/nodes/fixture/tasks" {
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `{"data":[
   {"upid":"done","type":"vzdump","status":"OK","endtime":20},
   {"upid":"failed","type":"vzdump","status":"backup failed","endtime":20},
   {"upid":"active","type":"vzdump","status":"running"},
   {"upid":"warning","type":"vzdump","status":"stopped","exitstatus":"WARNINGS: 1"},
   {"upid":"exit-only","type":"vzdump","exitstatus":"OK","endtime":20}
  ]}`)
	})
	tasks, err := p.Tasks(t.Context(), "fixture")
	if err != nil {
		t.Fatal(err)
	}
	backups, err := p.Backups(t.Context(), "fixture")
	if err != nil {
		t.Fatal(err)
	}
	states := []string{"stopped", "stopped", "running", "stopped", "stopped"}
	statuses := []string{"OK", "backup failed", "running", "WARNINGS: 1", "OK"}
	if len(tasks) != len(states) || len(backups) != len(states) {
		t.Fatal(tasks, backups)
	}
	for i, b := range backups {
		if b.State != states[i] || b.Status != statuses[i] || b.Node != "fixture" || b.State != tasks[i].State || b.Status != tasks[i].Status {
			t.Errorf("task=%+v backup=%+v want %s/%s", tasks[i], b, states[i], statuses[i])
		}
	}
}

func TestReportCephMissingBinaryIsUnsupported(t *testing.T) {
	for _, tc := range []struct {
		status      int
		detail      string
		unsupported bool
	}{
		{500, "binary not installed: /usr/bin/ceph-mon", true},
		{500, "unable to query cluster", false},
		{403, "binary not installed: /usr/bin/ceph-mon", false},
	} {
		t.Run(fmt.Sprintf("%d/%s", tc.status, tc.detail), func(t *testing.T) {
			p := connectForStorageTest(t, func(w http.ResponseWriter, r *http.Request) { http.Error(w, tc.detail, tc.status) })
			_, err := p.CephStatus(t.Context(), "fixture")
			if err == nil || errors.Is(err, app.ErrUnsupportedCap) != tc.unsupported {
				t.Fatalf("error=%v unsupported=%t", err, tc.unsupported)
			}
			if tc.unsupported && app.ExitCodeFromError(err) != app.ExitUnsupportedCap {
				t.Fatalf("unsupported exit = %d, want %d", app.ExitCodeFromError(err), app.ExitUnsupportedCap)
			}
			if !tc.unsupported {
				var pe *app.ProviderError
				if !errors.As(err, &pe) {
					t.Fatal(err)
				}
				if pe.StatusCode != tc.status && (tc.status != 500 || pe.StatusCode != 0) {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestReportClusterOnlineAndHAUnknown(t *testing.T) {
	p := connectForStorageTest(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/version":
			fmt.Fprint(w, `{"data":{"version":"9.2.21"}}`)
		case "/api2/json/nodes":
			fmt.Fprint(w, `{"data":[]}`)
		case "/api2/json/cluster/status":
			fmt.Fprint(w, `{"data":[{"type":"node","name":"up","online":1},{"type":"node","name":"down","online":"0"},{"type":"node","name":"unknown"},{"type":"node","name":"explicit","status":"maintenance","online":1}]}`)
		case "/api2/json/cluster/ha/status/current":
			fmt.Fprint(w, `{"data":[{"id":"quorum","type":"quorum","status":"OK"},{"id":"ct:9000","type":"service","state":"started"}]}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	})
	cluster, err := p.Cluster(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	details, err := p.ClusterStatuses(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{"online", "offline", "unknown", "maintenance"} {
		if cluster.NodeDetail[i].Status != want || details[i].Status != want {
			t.Errorf("node %d: %+v %+v want %s", i, cluster.NodeDetail[i], details[i], want)
		}
	}
	ha, err := p.HACurrent(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(ha) != 2 || ha[0].State != "unknown" || ha[0].Status != "OK" || ha[1].State != "started" {
		t.Fatal(ha)
	}
}
