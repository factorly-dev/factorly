package agent

import (
	"context"
	"sync"
	"time"
)

// Info holds identity and metadata for a connected agent.
type Info struct {
	ID           string // MCP session ID
	Name         string // Client name (e.g. "claude-code", "cursor")
	Version      string // Client version
	ConnectedAt  time.Time
	LastActivity time.Time
}

type contextKey struct{}

// WithAgentID stores the agent ID in the context.
func WithAgentID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, contextKey{}, id)
}

// AgentID retrieves the agent ID from context, or "" if not set.
func AgentID(ctx context.Context) string {
	id, _ := ctx.Value(contextKey{}).(string)
	return id
}

// Registry tracks connected agents.
type Registry struct {
	mu     sync.RWMutex
	agents map[string]*Info
}

// NewRegistry creates an agent registry.
func NewRegistry() *Registry {
	return &Registry{agents: make(map[string]*Info)}
}

// Register adds or updates an agent in the registry.
func (r *Registry) Register(info *Info) {
	r.mu.Lock()
	defer r.mu.Unlock()
	info.LastActivity = time.Now()
	if info.ConnectedAt.IsZero() {
		info.ConnectedAt = time.Now()
	}
	r.agents[info.ID] = info
}

// Touch updates the last activity time for an agent.
func (r *Registry) Touch(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a, ok := r.agents[id]; ok {
		a.LastActivity = time.Now()
	}
}

// Get returns agent info by ID, or nil if not found.
func (r *Registry) Get(id string) *Info {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.agents[id]
}

// List returns all registered agents.
func (r *Registry) List() []*Info {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*Info, 0, len(r.agents))
	for _, a := range r.agents {
		result = append(result, a)
	}
	return result
}

// Prune removes agents inactive for longer than maxAge.
func (r *Registry) Prune(maxAge time.Duration) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	cutoff := time.Now().Add(-maxAge)
	pruned := 0
	for id, a := range r.agents {
		if a.LastActivity.Before(cutoff) {
			delete(r.agents, id)
			pruned++
		}
	}
	return pruned
}
