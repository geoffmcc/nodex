package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/domain"
)

type mockNoDetail struct{ e2eMockProvider }

func (m *mockNoDetail) Capabilities() []domain.Capability {
	return []domain.Capability{domain.CapabilityNodes, domain.CapabilityVMs, domain.CapabilityContainers, domain.CapabilityStorage, domain.CapabilityCluster}
}

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
