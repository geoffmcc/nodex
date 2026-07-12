package provider

import (
	"fmt"
	"sync"

	"github.com/geoffmcc/nodex/internal/domain"
)

// Factory creates a new Provider instance.
type Factory func() domain.Provider

var (
	mu       sync.RWMutex
	registry = make(map[string]Factory)
)

// Register adds a provider factory to the registry.
func Register(name string, factory Factory) {
	mu.Lock()
	defer mu.Unlock()
	if _, exists := registry[name]; exists {
		panic(fmt.Sprintf("provider %q already registered", name))
	}
	registry[name] = factory
}

// Get returns a new Provider instance for the given name.
func Get(name string) (domain.Provider, error) {
	mu.RLock()
	defer mu.RUnlock()
	factory, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown provider: %q", name)
	}
	return factory(), nil
}

// List returns all registered provider names.
func List() []string {
	mu.RLock()
	defer mu.RUnlock()
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	return names
}

// IsRegistered returns true if a provider with the given name is registered.
func IsRegistered(name string) bool {
	mu.RLock()
	defer mu.RUnlock()
	_, ok := registry[name]
	return ok
}
