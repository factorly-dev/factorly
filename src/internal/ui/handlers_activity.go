// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/factorly-dev/factorly/internal/provider"
	"github.com/factorly-dev/factorly/internal/proxy"
)

// activityEvent wraps either a CallEvent or a StepEvent for the SSE stream.
type activityEvent struct {
	Type string // "call" or "workflow_step"
	Call *proxy.CallEvent
	Step *stepEventData
}

type stepEventData struct {
	Workflow string `json:"workflow"`
	provider.StepEvent
}

// ActivityBroadcaster fans out proxy CallEvents and workflow step events to all connected SSE clients.
type ActivityBroadcaster struct {
	mu      sync.Mutex
	clients map[chan activityEvent]bool
}

func NewActivityBroadcaster() *ActivityBroadcaster {
	return &ActivityBroadcaster{
		clients: make(map[chan activityEvent]bool),
	}
}

// Broadcast sends a call event to all connected clients.
func (b *ActivityBroadcaster) Broadcast(e proxy.CallEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.clients {
		select {
		case ch <- activityEvent{Type: "call", Call: &e}:
		default:
		}
	}
}

// BroadcastStep sends a workflow step event to all connected clients.
func (b *ActivityBroadcaster) BroadcastStep(workflow string, e provider.StepEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.clients {
		select {
		case ch <- activityEvent{Type: "workflow_step", Step: &stepEventData{Workflow: workflow, StepEvent: e}}:
		default:
		}
	}
}

func (b *ActivityBroadcaster) subscribe() chan activityEvent {
	ch := make(chan activityEvent, 32)
	b.mu.Lock()
	b.clients[ch] = true
	b.mu.Unlock()
	return ch
}

func (b *ActivityBroadcaster) unsubscribe(ch chan activityEvent) {
	b.mu.Lock()
	delete(b.clients, ch)
	b.mu.Unlock()
	close(ch)
}

func (s *Server) handleActivityStream(w http.ResponseWriter, r *http.Request) {
	if s.activity == nil {
		http.Error(w, "activity feed not available", http.StatusServiceUnavailable)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher.Flush()

	ch := s.activity.subscribe()
	defer s.activity.unsubscribe(ch)

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case ae, ok := <-ch:
			if !ok {
				return
			}
			var eventType string
			var data []byte
			switch ae.Type {
			case "workflow_step":
				eventType = "workflow_step"
				data, _ = json.Marshal(ae.Step)
			default:
				eventType = "call"
				e := ae.Call
				data, _ = json.Marshal(map[string]any{
					"timestamp":       e.Timestamp.Format("15:04:05"),
					"tool":            e.Tool,
					"status":          e.Status,
					"duration_ms":     e.DurationMs,
					"shadow":          e.ShadowAction,
					"agent_id":        e.AgentID,
					"error":           e.Error,
					"output":          e.Output,
					"params":          e.Params,
					"hash":            e.Hash,
					"workflow_run_id": e.WorkflowRunID,
					"workflow_name":   e.WorkflowName,
				})
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, data)
			flusher.Flush()
		}
	}
}

func (s *Server) handleActivity(w http.ResponseWriter, r *http.Request) {
	// The right-side activity drawer was replaced by /dashboard's
	// live feed in v0.15.0. Bookmarks and any /activity links from
	// older docs land on the dashboard instead.
	http.Redirect(w, r, "/dashboard", http.StatusFound)
}
