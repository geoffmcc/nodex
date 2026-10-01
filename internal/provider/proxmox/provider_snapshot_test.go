package proxmox

import (
	"context"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/provider/proxmox/client"
	"github.com/geoffmcc/nodex/internal/transport/httpclient"
)

// newSnapshotTestProvider connects a Provider to a test server serving payload.
func newSnapshotTestProvider(t *testing.T, payload string) *Provider {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/qemu/100/snapshot"),
			strings.HasSuffix(r.URL.Path, "/lxc/9610/snapshot"):
			_, _ = fmt.Fprint(w, payload)
		default:
			_, _ = fmt.Fprint(w, `{"data":{"version":"9.2.2","release":"9.2"}}`)
		}
	}))
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

// snapshotCtime must prefer "snaptime", which is what the Proxmox VE snapshot
// endpoint actually returns, while still honouring "ctime" from callers that
// construct the item directly.
func TestSnapshotCtimePrefersSnaptime(t *testing.T) {
	cases := []struct {
		name string
		item client.SnapshotListItem
		want int
	}{
		{name: "snaptime only", item: client.SnapshotListItem{Snaptime: 1790869735}, want: 1790869735},
		{name: "ctime fallback", item: client.SnapshotListItem{Ctime: 1700000000}, want: 1700000000},
		{name: "snaptime wins over ctime", item: client.SnapshotListItem{Snaptime: 2, Ctime: 1}, want: 2},
		{name: "current has neither", item: client.SnapshotListItem{Name: "current"}, want: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := snapshotCtime(tc.item); got != tc.want {
				t.Fatalf("snapshotCtime(%+v) = %d, want %d", tc.item, got, tc.want)
			}
		})
	}
}

// A description written by "snapshot create" must survive the read-back, and the
// creation time must be non-zero. Both fields are omitempty in JSON output, so
// before this fix they vanished entirely from `snapshots --output json` with
// exit code 0 and no warning (evidence 214/215 on container 9610).
func TestVMSnapshotsPreservesSnaptimeAndDescription(t *testing.T) {
	p := newSnapshotTestProvider(t, `{"data":[`+
		`{"description":"marker text for round-trip test\n","name":"nx4-desc","parent":"nx4-base","snaptime":1790869776},`+
		`{"description":"You are here!","name":"current","parent":"nx4-desc"}`+
		`]}`)

	snaps, err := p.VMSnapshots(context.Background(), "pve1", 100)
	if err != nil {
		t.Fatalf("VMSnapshots: %v", err)
	}
	if len(snaps) != 2 {
		t.Fatalf("len(snaps) = %d, want 2", len(snaps))
	}
	if snaps[0].Ctime != 1790869776 {
		t.Errorf("snaps[0].Ctime = %d, want 1790869776 (was silently zero)", snaps[0].Ctime)
	}
	if got := snaps[0].Description; got != "marker text for round-trip test\n" {
		t.Errorf("snaps[0].Description = %q, want the stored description", got)
	}
	if snaps[0].Node != "pve1" || snaps[0].Target != "pve1/100" {
		t.Errorf("snaps[0] node/target = %q/%q", snaps[0].Node, snaps[0].Target)
	}
	if snaps[1].Ctime != 0 {
		t.Errorf("snaps[1].Ctime = %d, want 0 for the current pseudo-snapshot", snaps[1].Ctime)
	}
}

// ContainerSnapshots must preserve the same fields; the bug affected both paths.
func TestContainerSnapshotsPreservesSnaptimeAndDescription(t *testing.T) {
	p := newSnapshotTestProvider(t, `{"data":[{"description":"no-description","name":"nx4fu-snap1","snaptime":1790869646}]}`)

	snaps, err := p.ContainerSnapshots(context.Background(), "pve1", 9610)
	if err != nil {
		t.Fatalf("ContainerSnapshots: %v", err)
	}
	if len(snaps) != 1 {
		t.Fatalf("len(snaps) = %d, want 1", len(snaps))
	}
	if snaps[0].Ctime != 1790869646 {
		t.Errorf("Ctime = %d, want 1790869646 (was silently zero)", snaps[0].Ctime)
	}
	if snaps[0].Description != "no-description" {
		t.Errorf("Description = %q, want no-description", snaps[0].Description)
	}
	if snaps[0].Target != "pve1/9610" {
		t.Errorf("Target = %q, want pve1/9610", snaps[0].Target)
	}
}
