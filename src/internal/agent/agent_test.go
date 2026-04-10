package agent

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	r.Register(&Info{ID: "sess-1", Name: "claude-code", Version: "1.0"})

	got := r.Get("sess-1")
	if got == nil {
		t.Fatal("expected agent, got nil")
	}
	if got.Name != "claude-code" {
		t.Errorf("expected name claude-code, got %s", got.Name)
	}
	if got.Version != "1.0" {
		t.Errorf("expected version 1.0, got %s", got.Version)
	}
	if got.ConnectedAt.IsZero() {
		t.Error("expected ConnectedAt to be set")
	}
	if got.LastActivity.IsZero() {
		t.Error("expected LastActivity to be set")
	}

	// Not found
	if r.Get("nonexistent") != nil {
		t.Error("expected nil for nonexistent agent")
	}
}

func TestAgentID(t *testing.T) {
	ctx := context.Background()
	if id := AgentID(ctx); id != "" {
		t.Errorf("expected empty string, got %q", id)
	}

	ctx = WithAgentID(ctx, "sess-42")
	if id := AgentID(ctx); id != "sess-42" {
		t.Errorf("expected sess-42, got %q", id)
	}
}

func TestTouch(t *testing.T) {
	r := NewRegistry()
	r.Register(&Info{ID: "sess-1", Name: "test"})

	before := r.Get("sess-1").LastActivity
	time.Sleep(2 * time.Millisecond)
	r.Touch("sess-1")
	after := r.Get("sess-1").LastActivity

	if !after.After(before) {
		t.Error("expected LastActivity to advance after Touch")
	}

	// Touch on nonexistent should not panic
	r.Touch("nonexistent")
}

func TestList(t *testing.T) {
	r := NewRegistry()
	r.Register(&Info{ID: "a", Name: "agent-a"})
	r.Register(&Info{ID: "b", Name: "agent-b"})

	list := r.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(list))
	}
}

func TestPrune(t *testing.T) {
	r := NewRegistry()
	r.Register(&Info{ID: "old", Name: "old-agent"})

	// Manually backdate LastActivity
	r.mu.Lock()
	r.agents["old"].LastActivity = time.Now().Add(-2 * time.Hour)
	r.mu.Unlock()

	r.Register(&Info{ID: "new", Name: "new-agent"})

	pruned := r.Prune(1 * time.Hour)
	if pruned != 1 {
		t.Errorf("expected 1 pruned, got %d", pruned)
	}
	if r.Get("old") != nil {
		t.Error("expected old agent to be pruned")
	}
	if r.Get("new") == nil {
		t.Error("expected new agent to remain")
	}
}

func TestConcurrentAccess(t *testing.T) {
	r := NewRegistry()
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			r.Register(&Info{ID: id, Name: "agent-" + id})
			r.Get(id)
			r.Touch(id)
			r.List()
		}(string(rune('A' + i%26)))
	}

	wg.Wait()

	// Just verify no panics occurred and registry is in valid state
	if len(r.List()) == 0 {
		t.Error("expected at least one agent after concurrent access")
	}
}
