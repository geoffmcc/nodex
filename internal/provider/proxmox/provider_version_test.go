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

func TestAPIVersionReportsConnectedProxmoxVersion(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api2/json/version" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{"data":{"version":"9.2.2","release":"9.2","repoid":"test"}}`)
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
	if got := p.APIVersion(); got != "" {
		t.Fatalf("APIVersion before health = %q, want empty", got)
	}
	if err := p.Health(context.Background()); err != nil {
		t.Fatalf("Health: %v", err)
	}
	if got := p.APIVersion(); got != "9.2.2" {
		t.Fatalf("APIVersion = %q, want 9.2.2", got)
	}
}

func TestNodeStatusFillsIdentityFromRequestedNode(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/pve-test/status":
			_, _ = fmt.Fprint(w, `{"data":{"cpu":0.1,"maxcpu":2,"uptime":60}}`)
		case "/api2/json/nodes":
			_, _ = fmt.Fprint(w, `{"data":[{"node":"pve-test","status":"online"}]}`)
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
	got, err := p.NodeStatus(context.Background(), "pve-test")
	if err != nil {
		t.Fatalf("NodeStatus: %v", err)
	}
	if got["id"] != "node/pve-test" || got["node"] != "pve-test" || got["type"] != "node" || got["status"] != "online" {
		t.Fatalf("identity = %#v", got)
	}
}
