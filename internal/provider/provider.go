// Package provider turns source-specific alert payloads (webhook bodies or
// CLI input files) into a NormalizedAlert. Each concrete provider registers
// itself via init() so the server and CLI can look it up by name.
package provider

import (
	"fmt"
	"sort"
	"sync"

	"github.com/ryuichi1208/asagao/internal/alert"
)

// Provider normalizes a raw payload from one monitoring SaaS into a
// NormalizedAlert. Implementations must be safe for concurrent use.
type Provider interface {
	// Name is the registry key, e.g. "datadog".
	Name() string
	// Normalize parses raw payload bytes into a NormalizedAlert.
	Normalize(raw []byte) (*alert.NormalizedAlert, error)
}

var (
	mu        sync.RWMutex
	providers = map[string]Provider{}
)

// Register adds p to the registry under p.Name(). It panics on duplicate
// names, which can only happen from a programming error at init time.
func Register(p Provider) {
	mu.Lock()
	defer mu.Unlock()
	name := p.Name()
	if _, dup := providers[name]; dup {
		panic(fmt.Sprintf("provider: %q registered twice", name))
	}
	providers[name] = p
}

// Get returns the provider registered under name.
func Get(name string) (Provider, bool) {
	mu.RLock()
	defer mu.RUnlock()
	p, ok := providers[name]
	return p, ok
}

// Names returns the sorted list of registered provider names.
func Names() []string {
	mu.RLock()
	defer mu.RUnlock()
	names := make([]string, 0, len(providers))
	for n := range providers {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
