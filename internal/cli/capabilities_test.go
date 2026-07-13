package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/domain"
)

// bareProvider implements only the base domain.Provider interface with no
// optional capabilities, suitable for embedding in mockNoDetail.
type bareProvider struct{}

func (b *bareProvider) Name() string                                                     { return "bare" }
func (b *bareProvider) Version() string                                                  { return "0" }
func (b *bareProvider) Connect(_ context.Context, _ string, _ *domain.Credentials) error { return nil }
func (b *bareProvider) Close() error                                                     { return nil }
func (b *bareProvider) Capabilities() []domain.Capability                                { return nil }
func (b *bareProvider) Nodes(_ context.Context) ([]domain.Node, error)                   { return nil, nil }
func (b *bareProvider) VMs(_ context.Context) ([]domain.VM, error)                       { return nil, nil }
func (b *bareProvider) Containers(_ context.Context) ([]domain.Container, error)         { return nil, nil }
func (b *bareProvider) Storage(_ context.Context) ([]domain.Storage, error)              { return nil, nil }
func (b *bareProvider) Cluster(_ context.Context) (*domain.Cluster, error)               { return nil, nil }
func (b *bareProvider) VMConfig(_ context.Context, _ string, _ int) (map[string]interface{}, error) {
	return nil, nil
}
func (b *bareProvider) ContainerConfig(_ context.Context, _ string, _ int) (map[string]interface{}, error) {
	return nil, nil
}
func (b *bareProvider) StorageContent(_ context.Context, _, _ string) ([]domain.StorageContentItem, error) {
	return nil, nil
}
func (b *bareProvider) Tasks(_ context.Context, _ string) ([]domain.Task, error)  { return nil, nil }
func (b *bareProvider) Task(_ context.Context, _, _ string) (*domain.Task, error) { return nil, nil }
func (b *bareProvider) VMSnapshots(_ context.Context, _ string, _ int) ([]domain.Snapshot, error) {
	return nil, nil
}
func (b *bareProvider) ContainerSnapshots(_ context.Context, _ string, _ int) ([]domain.Snapshot, error) {
	return nil, nil
}
func (b *bareProvider) Events(_ context.Context) ([]domain.Event, error) { return nil, nil }
func (b *bareProvider) Syslog(_ context.Context, _ string) ([]domain.SyslogEntry, error) {
	return nil, nil
}
func (b *bareProvider) Backups(_ context.Context, _ string) ([]domain.Backup, error) { return nil, nil }
func (b *bareProvider) FirewallRules(_ context.Context) ([]domain.FirewallRule, error) {
	return nil, nil
}
func (b *bareProvider) HAResources(_ context.Context) ([]domain.HAResource, error) { return nil, nil }
func (b *bareProvider) HAGroups(_ context.Context) ([]domain.HAGroup, error)       { return nil, nil }

type mockNoDetail struct{ bareProvider }

func TestRequireNodeDetailReturnsError(t *testing.T) {
	_, err := requireNodeDetail(&mockNoDetail{})
	if err == nil {
		t.Fatal("expected error for unsupported capability")
	}
	if !strings.Contains(err.Error(), "unsupported capability") {
		t.Errorf("error = %q, want 'unsupported capability'", err.Error())
	}
	var ec *app.ExitCoder
	if !strings.Contains(err.Error(), "node detail") {
		t.Errorf("error = %q, want 'node detail'", err.Error())
	}
	_ = ec
}

type mockDetailProvider struct {
	e2eMockProvider
}

func (m *mockDetailProvider) NodeStatus(_ context.Context, _ string) (map[string]interface{}, error) {
	return nil, nil
}
func (m *mockDetailProvider) NodeServices(_ context.Context, _ string) ([]domain.NodeService, error) {
	return nil, nil
}
func (m *mockDetailProvider) NodeNetwork(_ context.Context, _ string) ([]domain.NodeNetwork, error) {
	return nil, nil
}
func (m *mockDetailProvider) NodeDNS(_ context.Context, _ string) (*domain.NodeDNS, error) {
	return nil, nil
}
func (m *mockDetailProvider) NodeTime(_ context.Context, _ string) (*domain.NodeTime, error) {
	return nil, nil
}
func (m *mockDetailProvider) NodeDisks(_ context.Context, _ string) ([]domain.NodeDisk, error) {
	return nil, nil
}
func (m *mockDetailProvider) NodeCertificates(_ context.Context, _ string) ([]domain.NodeCertificate, error) {
	return nil, nil
}
func (m *mockDetailProvider) NodeSubscription(_ context.Context, _ string) (*domain.NodeSubscription, error) {
	return nil, nil
}
func (m *mockDetailProvider) NodeUpdates(_ context.Context, _ string) ([]domain.NodeUpdate, error) {
	return nil, nil
}

func TestRequireNodeDetailSucceeds(t *testing.T) {
	_, err := requireNodeDetail(&mockDetailProvider{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRequireFirewallAdvancedReturnsError(t *testing.T) {
	_, err := requireFirewallAdvanced(&mockNoDetail{})
	if err == nil {
		t.Fatal("expected error for unsupported capability")
	}
}

func TestRequireHAStatusReturnsError(t *testing.T) {
	_, err := requireHAStatus(&mockNoDetail{})
	if err == nil {
		t.Fatal("expected error for unsupported capability")
	}
}

func TestRequireBackupContentReturnsError(t *testing.T) {
	_, err := requireBackupContent(&mockNoDetail{})
	if err == nil {
		t.Fatal("expected error for unsupported capability")
	}
}

func TestRequireSDNReturnsError(t *testing.T) {
	_, err := requireSDN(&mockNoDetail{})
	if err == nil {
		t.Fatal("expected error for unsupported capability")
	}
}

func TestRequireSnapshotDetailReturnsError(t *testing.T) {
	_, err := requireSnapshotDetail(&mockNoDetail{})
	if err == nil {
		t.Fatal("expected error for unsupported capability")
	}
}
