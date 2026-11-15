package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/config"
	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/provider"
)

var downloadMock *reviewDownloadProvider

func init() {
	provider.Register("review-download-mock", func() domain.Provider { return downloadMock })
}

type reviewDownloadProvider struct {
	*e2eMockProvider
	connections, downloads int
	duringTransfer         func() error
}

func (p *reviewDownloadProvider) Connect(context.Context, string, *domain.Credentials) error {
	p.connections++
	return nil
}
func (p *reviewDownloadProvider) UploadContent(context.Context, string, string, string) (string, error) {
	return "", nil
}
func (p *reviewDownloadProvider) DeleteContent(context.Context, string, string, string) (string, error) {
	return "", nil
}
func (p *reviewDownloadProvider) DownloadContentBody(_ context.Context, _, _, _ string, w io.Writer) error {
	p.downloads++
	if p.duringTransfer != nil {
		if err := p.duringTransfer(); err != nil {
			return err
		}
	}
	_, err := io.WriteString(w, "new")
	return err
}
func TestStorageDownloadCommitAndConfirmation(t *testing.T) {
	isolateConfigAndHome(t)
	setupE2EConfig(t)
	cfg, err := config.Read()
	if err != nil {
		t.Fatal(err)
	}
	p := cfg.Profiles["e2e"]
	p.Provider = "review-download-mock"
	cfg.Profiles["e2e"] = p
	path, err := config.ConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if err = config.WriteTo(cfg, path); err != nil {
		t.Fatal(err)
	}
	downloadMock = &reviewDownloadProvider{e2eMockProvider: &e2eMockProvider{}}
	t.Cleanup(func() { downloadMock = nil })
	dest := filepath.Join(t.TempDir(), "volume.iso")
	if err := os.WriteFile(dest, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	run := func(flags ...string) error {
		var out, stderr bytes.Buffer
		args := append(flags, "storage", "download", "node", "store", "vol", dest)
		return Run(context.Background(), args, &out, &stderr)
	}
	err = run("--force", "--non-interactive")
	if err == nil || app.ExitCodeFromError(err) != app.ExitUsage || !strings.Contains(err.Error(), "confirmation") {
		t.Fatalf("expected confirmation usage error, got %v", err)
	}
	if downloadMock.connections != 0 || downloadMock.downloads != 0 {
		t.Fatalf("unexpected provider calls: %+v", downloadMock)
	}
	check := func(want string) {
		t.Helper()
		got, err := os.ReadFile(dest)
		if err != nil || string(got) != want {
			t.Fatalf("destination %q, %v; want %q", got, err, want)
		}
	}
	check("old")
	if err := run("--force", "--yes"); err != nil {
		t.Fatal(err)
	}
	check("new")
	if downloadMock.connections != 1 || downloadMock.downloads != 1 {
		t.Fatal("expected one connection and download")
	}
	if err := os.Remove(dest); err != nil {
		t.Fatal(err)
	}
	downloadMock.duringTransfer = func() error { return os.WriteFile(dest, []byte("arrived during transfer"), 0600) }
	if err := run("--yes"); err == nil {
		t.Fatal("expected no-replace failure")
	}
	check("arrived during transfer")
}
