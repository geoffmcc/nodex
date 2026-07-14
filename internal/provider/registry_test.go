package provider

import (
	"context"
	"testing"

	"github.com/geoffmcc/nodex/internal/domain"
)

type mockProvider struct {
	name string
}

func (m *mockProvider) Name() string                                                     { return m.name }
func (m *mockProvider) Version() string                                                  { return "0.1.0" }
func (m *mockProvider) Connect(_ context.Context, _ string, _ *domain.Credentials) error { return nil }
func (m *mockProvider) Close() error                                                     { return nil }
func (m *mockProvider) Health(_ context.Context) error                                   { return nil }
func (m *mockProvider) Capabilities() []domain.Capability                                { return nil }

func TestRegisterAndGet(t *testing.T) {
	factory := func() domain.Provider { return &mockProvider{name: "test-provider"} }
	Register("test-provider", factory)

	prov, err := Get("test-provider")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if prov.Name() != "test-provider" {
		t.Errorf("Name() = %q, want %q", prov.Name(), "test-provider")
	}
}

func TestGetUnknown(t *testing.T) {
	_, err := Get("nonexistent-provider")
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestList(t *testing.T) {
	names := List()
	if len(names) == 0 {
		t.Fatal("expected at least one registered provider")
	}
}

func TestIsRegistered(t *testing.T) {
	if !IsRegistered("test-provider") {
		t.Error("expected test-provider to be registered")
	}
	if IsRegistered("nonexistent-provider") {
		t.Error("expected nonexistent-provider to not be registered")
	}
}
