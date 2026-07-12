package provider

import (
	"testing"

	"github.com/geoffmcc/nodex/internal/domain"
)

func TestCapabilityNames(t *testing.T) {
	caps := []domain.Capability{
		domain.CapabilityNodes,
		domain.CapabilityVMs,
		domain.CapabilityContainers,
		domain.CapabilityStorage,
		domain.CapabilityCluster,
	}

	expected := []string{"nodes", "vms", "containers", "storage", "cluster"}
	for i, c := range caps {
		if string(c) != expected[i] {
			t.Errorf("Capability %d = %q, want %q", i, string(c), expected[i])
		}
	}
}

func TestCapabilitySet_Has(t *testing.T) {
	s := NewCapabilitySet(domain.CapabilityNodes, domain.CapabilityVMs)
	if !s.Has(domain.CapabilityNodes) {
		t.Error("expected Has(Nodes) = true")
	}
	if !s.Has(domain.CapabilityVMs) {
		t.Error("expected Has(VMs) = true")
	}
	if s.Has(domain.CapabilityStorage) {
		t.Error("expected Has(Storage) = false")
	}
}

func TestCapabilitySet_List(t *testing.T) {
	s := NewCapabilitySet(domain.CapabilityNodes, domain.CapabilityStorage)
	list := s.List()
	if len(list) != 2 {
		t.Errorf("List returned %d, want 2", len(list))
	}
}

func TestCapabilitySet_Supports(t *testing.T) {
	s := NewCapabilitySet(domain.CapabilityNodes, domain.CapabilityVMs, domain.CapabilityStorage)
	if !s.Supports(domain.CapabilityNodes, domain.CapabilityStorage) {
		t.Error("expected Supports(Nodes, Storage) = true")
	}
	if s.Supports(domain.CapabilityNodes, domain.CapabilityCluster) {
		t.Error("expected Supports(Nodes, Cluster) = false")
	}
}
