package proxmox

import (
	"context"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/transport/httpclient"
)

func TestClusterIncludesStandaloneNodeIdentity(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/version":
			_, _ = fmt.Fprint(w, `{"data":{"version":"9.2.11","release":"9.2","repoid":"test"}}`)
		case "/api2/json/nodes":
			_, _ = fmt.Fprint(w, `{"data":[{"node":"proxmox","status":"online"}]}`)
		case "/api2/json/cluster/status":
			_, _ = fmt.Fprint(w, `{"data":[{"type":"node","id":"node/proxmox","name":"proxmox","status":"online","ip":"10.0.0.10","version":9}]}`)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

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

	got, err := p.Cluster(context.Background())
	if err != nil {
		t.Fatalf("Cluster: %v", err)
	}
	if got.Name != "proxmox" || !got.Standalone {
		t.Errorf("identity = name %q standalone %t, want proxmox standalone", got.Name, got.Standalone)
	}
	if got.Version != "9.2.11" {
		t.Errorf("Version = %q, want API version 9.2.11", got.Version)
	}
	if got.Nodes != 1 {
		t.Errorf("Nodes = %d, want 1", got.Nodes)
	}
	if got.Quorate != nil {
		t.Errorf("Quorate = %v, want unknown for a standalone node", *got.Quorate)
	}
	if len(got.NodeDetail) != 1 {
		t.Fatalf("NodeDetail has %d entries, want 1", len(got.NodeDetail))
	}
	node := got.NodeDetail[0]
	if node.Name != "proxmox" || node.IP != "10.0.0.10" || node.Status != "online" {
		t.Errorf("NodeDetail[0] = %+v, want node identity, address, and status", node)
	}
}
