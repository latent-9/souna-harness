package provider

import (
	"fmt"
	"sort"
)

// Registry holds the configured providers.
type Registry struct {
	providers map[string]Provider
}

// NewRegistry builds a registry from config using build to construct each
// concrete provider.
func NewRegistry(cfg Config, build func(name string, pc ProviderConfig) (Provider, error)) (*Registry, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	r := &Registry{providers: make(map[string]Provider, len(cfg.Providers))}
	for name, pc := range cfg.Providers {
		p, err := build(name, pc)
		if err != nil {
			return nil, fmt.Errorf("provider %q: %w", name, err)
		}
		r.providers[name] = p
	}
	return r, nil
}

// Get returns the named provider.
func (r *Registry) Get(name string) (Provider, error) {
	p, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("provider %q not configured", name)
	}
	return p, nil
}

// Names returns the configured provider names, sorted.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
