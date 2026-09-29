package proxmox

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/provider/proxmox/client"
	"github.com/geoffmcc/nodex/internal/transport/httpclient"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// fakeSFTP satisfies sftpClient and records the dial/Open sequence.
type fakeSFTP struct {
	content map[string]string
	opened  []string
	openErr error
}

func (f *fakeSFTP) Open(path string) (io.ReadCloser, error) {
	f.opened = append(f.opened, path)
	if f.openErr != nil {
		return nil, f.openErr
	}
	c, ok := f.content[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return io.NopCloser(strings.NewReader(c)), nil
}

func (f *fakeSFTP) Close() error { return nil }

type dialRecord struct {
	host, user, keyFile string
	port                int
}

// newDownloadProvider builds a Provider connected to a TLS test server whose
// certificate is trusted via a CA file, like the rest of the provider tests.
func newDownloadProvider(t *testing.T, handler http.HandlerFunc) *Provider {
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

	creds := &domain.Credentials{Type: "token", TokenID: "root@pam!test", TokenSecret: "secret"}
	c, err := client.New(server.URL, creds, caOpt)
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	return &Provider{client: c}
}

// volumePathHandler serves the content/{volume} info response.
func volumePathHandler(path string, status int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if status != http.StatusOK {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"errors":[{"message":"volume not found"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"path":"` + path + `","format":"iso","size":1024}}`))
	}
}

// TestDownloadContentBodyRequiresSSHConfig verifies a clear error when the
// profile lacks the SSH configuration needed for SFTP.
func TestDownloadContentBodyRequiresSSHConfig(t *testing.T) {
	p := newDownloadProvider(t, volumePathHandler("/var/lib/vz/x.iso", http.StatusOK))

	var buf strings.Builder
	err := p.DownloadContentBody(context.Background(), "node1", "local", "local:iso/x.iso", &buf)
	if err == nil || !strings.Contains(err.Error(), "ssh_user") {
		t.Fatalf("error = %v, want ssh_user requirement", err)
	}

	p.SetSSHConfig("", "root", "", 0)
	err = p.DownloadContentBody(context.Background(), "node1", "local", "local:iso/x.iso", &buf)
	if err == nil || !strings.Contains(err.Error(), "ssh_key_file") {
		t.Fatalf("error = %v, want ssh_key_file requirement", err)
	}
}

// TestDownloadContentBodySFTPFlow verifies the full flow: volume path is
// resolved via the API, then streamed over the (fake) SFTP connection with the
// profile SSH settings.
func TestDownloadContentBodySFTPFlow(t *testing.T) {
	const remotePath = "/var/lib/vz/template/iso/x.iso"
	p := newDownloadProvider(t, volumePathHandler(remotePath, http.StatusOK))

	fake := &fakeSFTP{content: map[string]string{remotePath: "file-bytes"}}
	var rec dialRecord
	p.sftpDial = func(_ context.Context, host, user, keyFile string, port int) (sftpClient, error) {
		rec = dialRecord{host: host, user: user, keyFile: keyFile, port: port}
		return fake, nil
	}
	p.SetSSHConfig("", "root", "/home/geoffrey/.ssh/nodex-pve-production", 0)

	var buf strings.Builder
	if err := p.DownloadContentBody(context.Background(), "node1", "local", "local:iso/x.iso", &buf); err != nil {
		t.Fatalf("DownloadContentBody: %v", err)
	}
	if buf.String() != "file-bytes" {
		t.Fatalf("downloaded = %q, want file-bytes", buf.String())
	}
	if rec.host != "127.0.0.1" {
		t.Fatalf("dial host = %q, want endpoint hostname", rec.host)
	}
	if rec.user != "root" || rec.keyFile != "/home/geoffrey/.ssh/nodex-pve-production" || rec.port != 22 {
		t.Fatalf("dial args = %+v, want root key with port 22", rec)
	}
	if len(fake.opened) != 1 || fake.opened[0] != remotePath {
		t.Fatalf("opened = %v, want %q", fake.opened, remotePath)
	}
}

// TestDownloadContentBodyHostOverride verifies ssh_host overrides the endpoint
// hostname as the dial target.
func TestDownloadContentBodyHostOverride(t *testing.T) {
	p := newDownloadProvider(t, volumePathHandler("/var/lib/vz/x.iso", http.StatusOK))

	fake := &fakeSFTP{content: map[string]string{"/var/lib/vz/x.iso": "x"}}
	var rec dialRecord
	p.sftpDial = func(_ context.Context, host, user, keyFile string, port int) (sftpClient, error) {
		rec = dialRecord{host: host, user: user, keyFile: keyFile, port: port}
		return fake, nil
	}
	p.SetSSHConfig("10.0.0.99", "root", "/key", 2222)

	var buf strings.Builder
	if err := p.DownloadContentBody(context.Background(), "node1", "local", "local:iso/x.iso", &buf); err != nil {
		t.Fatalf("DownloadContentBody: %v", err)
	}
	if rec.host != "10.0.0.99" {
		t.Fatalf("dial host = %q, want ssh_host override", rec.host)
	}
	if rec.port != 2222 {
		t.Fatalf("dial port = %d, want 2222", rec.port)
	}
}

// TestDownloadContentBodyVolumePathError verifies that an API failure resolving
// the volume path aborts before any SFTP dial attempt.
func TestDownloadContentBodyVolumePathError(t *testing.T) {
	p := newDownloadProvider(t, volumePathHandler("", http.StatusNotFound))

	dialed := false
	p.sftpDial = func(_ context.Context, host, user, keyFile string, port int) (sftpClient, error) {
		dialed = true
		return &fakeSFTP{}, nil
	}
	p.SetSSHConfig("", "root", "/key", 0)

	var buf strings.Builder
	err := p.DownloadContentBody(context.Background(), "node1", "local", "local:iso/x.iso", &buf)
	if err == nil || !strings.Contains(err.Error(), "resolve volume path") {
		t.Fatalf("error = %v, want volume path resolution failure", err)
	}
	if dialed {
		t.Fatal("SFTP dial must not run when volume path resolution fails")
	}
}

// TestDownloadViaSFTPContextCancellation verifies an already-cancelled context
// aborts the transfer.
func TestDownloadViaSFTPContextCancellation(t *testing.T) {
	fake := &fakeSFTP{content: map[string]string{"/x.iso": "data"}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var buf strings.Builder
	err := downloadViaSFTP(ctx, "host", "root", "/key", 22, "/x.iso", &buf, func(_ context.Context, h, u, k string, p int) (sftpClient, error) {
		return fake, nil
	})
	if err == nil {
		t.Fatal("expected cancellation error")
	}
}

// TestDownloadViaSFTPOpenError verifies remote open failures surface.
func TestDownloadViaSFTPOpenError(t *testing.T) {
	fake := &fakeSFTP{openErr: os.ErrNotExist}
	var buf strings.Builder
	err := downloadViaSFTP(context.Background(), "host", "root", "/key", 22, "/missing.iso", &buf, func(_ context.Context, h, u, k string, p int) (sftpClient, error) {
		return fake, nil
	})
	if err == nil || !strings.Contains(err.Error(), "open remote file") {
		t.Fatalf("error = %v, want remote open failure", err)
	}
}

// TestRealSFTPDialRejectsBadKey verifies key loading/parsing failures without
// touching the network.
func TestRealSFTPDialRejectsBadKey(t *testing.T) {
	dir := t.TempDir()

	if _, err := realSFTPDial(context.Background(), "10.0.0.1", "root", filepath.Join(dir, "missing.key"), 22); err == nil {
		t.Fatal("expected error for missing key file")
	}

	bad := filepath.Join(dir, "bad.key")
	if err := os.WriteFile(bad, []byte("not a private key"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := realSFTPDial(context.Background(), "10.0.0.1", "root", bad, 22); err == nil {
		t.Fatal("expected error for unparseable key file")
	}
}

func TestNodesEnrichesIPFromClusterStatus(t *testing.T) {
	p := newDownloadProvider(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes":
			_, _ = io.WriteString(w, `{"data":[{"id":"node/pve1","node":"pve1","status":"online","type":"node"}]}`)
		case "/api2/json/cluster/status":
			_, _ = io.WriteString(w, `{"data":[{"type":"node","id":"node/pve1","name":"pve1","status":"online","ip":"192.0.2.21"}]}`)
		default:
			t.Errorf("unexpected request path %q", r.URL.Path)
			http.NotFound(w, r)
		}
	})
	defer p.Close()

	nodes, err := p.Nodes(context.Background())
	if err != nil {
		t.Fatalf("Nodes: %v", err)
	}
	if len(nodes) != 1 || nodes[0].IP != "192.0.2.21" {
		t.Fatalf("Nodes = %+v, want node IP 192.0.2.21", nodes)
	}
}

func TestRealSFTPDialRejectsOverlyReadableKey(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose POSIX file permission bits")
	}
	keyPath := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(keyPath, []byte("not a private key"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := realSFTPDial(context.Background(), "192.0.2.1", "root", keyPath, 22); err == nil || !strings.Contains(err.Error(), "permissions") {
		t.Fatalf("realSFTPDial error = %v, want insecure permissions before key parsing/network", err)
	}
}

func TestHostKeyCallbackRequiresKnownHost(t *testing.T) {
	makeSigner := func() ssh.Signer {
		t.Helper()
		_, privateKey, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		signer, err := ssh.NewSignerFromKey(privateKey)
		if err != nil {
			t.Fatal(err)
		}
		return signer
	}

	trusted := makeSigner()
	untrusted := makeSigner()
	knownHostsPath := filepath.Join(t.TempDir(), "known_hosts")
	line := knownhosts.Line([]string{"pve.example.test"}, trusted.PublicKey())
	if err := os.WriteFile(knownHostsPath, []byte(line+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	callback, err := hostKeyCallback(knownHostsPath)
	if err != nil {
		t.Fatalf("hostKeyCallback: %v", err)
	}
	remote := &net.TCPAddr{IP: net.ParseIP("192.0.2.20"), Port: 22}
	if err := callback("pve.example.test:22", remote, trusted.PublicKey()); err != nil {
		t.Fatalf("known server key rejected: %v", err)
	}
	if err := callback("pve.example.test:22", remote, untrusted.PublicKey()); err == nil {
		t.Fatal("unknown server key was accepted")
	}

	nonstandardPath := filepath.Join(t.TempDir(), "known_hosts")
	nonstandardLine := knownhosts.Line([]string{"[pve.example.test]:2222"}, trusted.PublicKey())
	if err := os.WriteFile(nonstandardPath, []byte(nonstandardLine+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	nonstandardCallback, err := hostKeyCallback(nonstandardPath)
	if err != nil {
		t.Fatalf("hostKeyCallback for nonstandard port: %v", err)
	}
	nonstandardRemote := &net.TCPAddr{IP: net.ParseIP("192.0.2.20"), Port: 2222}
	if err := nonstandardCallback("pve.example.test:2222", nonstandardRemote, trusted.PublicKey()); err != nil {
		t.Fatalf("known server key on nonstandard port rejected: %v", err)
	}
}

func TestHostKeyCallbackRequiresKnownHostsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing_known_hosts")
	if _, err := hostKeyCallback(path); err == nil {
		t.Fatal("missing known_hosts file accepted")
	}
}
