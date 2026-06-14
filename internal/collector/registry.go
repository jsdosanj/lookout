package collector

import (
	"fmt"
	"sort"
	"sync"
)

// Registry holds the collectors available to the agent. Collectors self-register
// from their package init() (one package per collector, C1..C12), so the agent
// binary's collector set is determined by which packages are imported. The
// scheduler reads the registry to plan budgeted, capability-gated runs.
//
// STUB: the surface is the contract; the collector agent wires real collectors
// into it. Concurrency-safe so init()-time registration is data-race free.
type Registry struct {
	mu         sync.RWMutex
	collectors map[string]Collector
}

// defaultRegistry is the process-wide registry collectors register into via
// init(). Use Register / Default to access it.
var defaultRegistry = &Registry{collectors: make(map[string]Collector)}

// Register adds a collector to the default registry, keyed by its Meta().ID.
// Intended to be called from a collector package's init(). Panics on a
// duplicate id (a programming error: two collectors claimed the same id).
func Register(c Collector) {
	id := c.Meta().ID
	defaultRegistry.mu.Lock()
	defer defaultRegistry.mu.Unlock()
	if _, exists := defaultRegistry.collectors[id]; exists {
		panic(fmt.Sprintf("collector: duplicate registration for id %q", id))
	}
	defaultRegistry.collectors[id] = c
}

// Default returns the process-wide registry.
func Default() *Registry { return defaultRegistry }

// Get returns the collector with the given id, or false if absent.
func (r *Registry) Get(id string) (Collector, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.collectors[id]
	return c, ok
}

// All returns the registered collectors sorted by id (stable ordering for the
// scheduler and for deterministic policy application).
func (r *Registry) All() []Collector {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.collectors))
	for id := range r.collectors {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]Collector, 0, len(ids))
	for _, id := range ids {
		out = append(out, r.collectors[id])
	}
	return out
}
