package proxmox

import (
	"context"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/transport/httpclient"
)

func TestCephInventoryReportsUnsupportedWhenCephIsNotInstalled(t *testing.T) {
	tests := []struct {
		name string
		path string
		call func(context.Context, *Provider) error
	}{
		{
			name: "status",
			path: "/api2/json/nodes/pve-test/ceph/status",
			call: func(ctx context.Context, p *Provider) error {
				_, err := p.CephStatus(ctx, "pve-test")
				return err
			},
		},
		{
			name: "monitors",
			path: "/api2/json/nodes/pve-test/ceph/mon",
			call: func(ctx context.Context, p *Provider) error {
				_, err := p.CephMONs(ctx, "pve-test")
				return err
			},
		},
		{
			name: "osds",
			path: "/api2/json/nodes/pve-test/ceph/osd",
			call: func(ctx context.Context, p *Provider) error {
				_, err := p.CephOSDs(ctx, "pve-test")
				return err
			},
		},
		{
			name: "pools",
			path: "/api2/json/nodes/pve-test/ceph/pool",
			call: func(ctx context.Context, p *Provider) error {
				_, err := p.CephPools(ctx, "pve-test")
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tt.path {
					t.Errorf("request path = %q, want %q", r.URL.Path, tt.path)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = fmt.Fprint(w, `{"message":"binary not installed: /usr/bin/ceph-mon"}`)
			}))
			defer server.Close()

			certPath := filepath.Join(t.TempDir(), "ca.pem")
			cert := server.Certificate()
			if cert == nil {
				t.Fatal("test server certificate is nil")
			}
			pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
			if err := os.WriteFile(certPath, pemBytes, 0o600); err != nil {
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

			err = tt.call(context.Background(), p)
			if !errors.Is(err, app.ErrUnsupportedCap) {
				t.Fatalf("error = %v, want unsupported capability", err)
			}
			if got := app.ExitCodeFromError(err); got != app.ExitUnsupportedCap {
				t.Fatalf("exit code = %d, want %d", got, app.ExitUnsupportedCap)
			}
		})
	}
}
