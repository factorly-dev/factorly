// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/factorly-dev/factorly/internal/proxy"
)

// ActivityBroadcaster fans out proxy CallEvents to all connected SSE clients.
type ActivityBroadcaster struct {
	mu      sync.Mutex
	clients map[chan proxy.CallEvent]bool
}

func NewActivityBroadcaster() *ActivityBroadcaster {
	return &ActivityBroadcaster{
		clients: make(map[chan proxy.CallEvent]bool),
	}
}

// Broadcast sends an event to all connected clients.
func (b *ActivityBroadcaster) Broadcast(e proxy.CallEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.clients {
		select {
		case ch <- e:
		default:
			// Client too slow, skip
		}
	}
}

func (b *ActivityBroadcaster) subscribe() chan proxy.CallEvent {
	ch := make(chan proxy.CallEvent, 32)
	b.mu.Lock()
	b.clients[ch] = true
	b.mu.Unlock()
	return ch
}

func (b *ActivityBroadcaster) unsubscribe(ch chan proxy.CallEvent) {
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
		case event, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(map[string]any{
				"timestamp":   event.Timestamp.Format("15:04:05"),
				"tool":        event.Tool,
				"status":      event.Status,
				"duration_ms": event.DurationMs,
				"shadow":      event.ShadowAction,
				"agent_id":    event.AgentID,
				"error":       event.Error,
				"output":      event.Output,
				"params":      event.Params,
			})
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

func (s *Server) handleActivity(w http.ResponseWriter, r *http.Request) {
	s.render(w, "activity.html", map[string]any{
		"Title": "Activity",
		"Nav":   "activity",
	})
}
