package provider

import "github.com/geoffmcc/nodex/internal/domain"

// CapabilitySet is a set of capabilities.
type CapabilitySet map[domain.Capability]bool

// NewCapabilitySet creates a CapabilitySet from a list of capabilities.
func NewCapabilitySet(caps ...domain.Capability) CapabilitySet {
	s := make(CapabilitySet, len(caps))
	for _, c := range caps {
		s[c] = true
	}
	return s
}

// Has returns true if the set contains the given capability.
func (s CapabilitySet) Has(cap domain.Capability) bool {
	return s[cap]
}

// List returns all capabilities in the set.
func (s CapabilitySet) List() []domain.Capability {
	caps := make([]domain.Capability, 0, len(s))
	for c := range s {
		caps = append(caps, c)
	}
	return caps
}

// Supports returns true if the set contains all given capabilities.
func (s CapabilitySet) Supports(caps ...domain.Capability) bool {
	for _, c := range caps {
		if !s[c] {
			return false
		}
	}
	return true
}
