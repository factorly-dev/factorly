// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
)

// ConfirmRequest is a pending shadow confirmation prompt.
type ConfirmRequest struct {
	ID       string            `json:"id"`
	Tool     string            `json:"tool"`
	Params   map[string]string `json:"params"`
	response chan bool
}

// ConfirmBroker manages pending confirmation prompts between the shadow
// policy and the browser UI.
type ConfirmBroker struct {
	mu      sync.Mutex
	pending map[string]*ConfirmRequest
	notify  chan struct{} // signaled when a new request arrives
	counter atomic.Int64
}

func NewConfirmBroker() *ConfirmBroker {
	return &ConfirmBroker{
		pending: make(map[string]*ConfirmRequest),
		notify:  make(chan struct{}, 1),
	}
}

// Request creates a confirm prompt and blocks until the browser responds
// or the context is cancelled. Returns true if approved.
func (b *ConfirmBroker) Request(ctx context.Context, toolName string, params map[string]string) bool {
	id := fmt.Sprintf("confirm-%d", b.counter.Add(1))
	req := &ConfirmRequest{
		ID:       id,
		Tool:     toolName,
		Params:   params,
		response: make(chan bool, 1),
	}

	b.mu.Lock()
	b.pending[id] = req
	b.mu.Unlock()

	// Signal SSE listeners that there's a new prompt
	select {
	case b.notify <- struct{}{}:
	default:
	}

	defer func() {
		b.mu.Lock()
		delete(b.pending, id)
		b.mu.Unlock()
	}()

	select {
	case approved := <-req.response:
		return approved
	case <-ctx.Done():
		return false
	}
}

// Respond resolves a pending confirmation.
func (b *ConfirmBroker) Respond(id string, approved bool) bool {
	b.mu.Lock()
	req, ok := b.pending[id]
	b.mu.Unlock()
	if !ok {
		return false
	}
	select {
	case req.response <- approved:
		return true
	default:
		return false
	}
}

// Pending returns all pending confirmation requests.
func (b *ConfirmBroker) Pending() []*ConfirmRequest {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []*ConfirmRequest
	for _, r := range b.pending {
		out = append(out, r)
	}
	return out
}

// Notify returns a channel that is signaled when new confirm requests arrive.
func (b *ConfirmBroker) Notify() <-chan struct{} {
	return b.notify
}

// --- HTTP handlers ---

func (s *Server) handleConfirmPending(w http.ResponseWriter, r *http.Request) {
	if s.confirmBroker == nil {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
		return
	}
	pending := s.confirmBroker.Pending()
	if pending == nil {
		pending = []*ConfirmRequest{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(pending)
}

func (s *Server) handleConfirmRespond(w http.ResponseWriter, r *http.Request) {
	if s.confirmBroker == nil {
		http.Error(w, "no confirm broker", http.StatusServiceUnavailable)
		return
	}

	id := r.PathValue("id")
	action := r.PathValue("action")

	approved := action == "approve"
	if ok := s.confirmBroker.Respond(id, approved); !ok {
		http.Error(w, "confirm request not found or already resolved", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if approved {
		fmt.Fprint(w, `<span class="text-green-600 text-xs">Approved</span>`)
	} else {
		fmt.Fprint(w, `<span class="text-red-600 text-xs">Denied</span>`)
	}
}

// handleConfirmSSE streams pending confirmations to the browser as SSE events.
func (s *Server) handleConfirmSSE(w http.ResponseWriter, r *http.Request) {
	if s.confirmBroker == nil {
		http.Error(w, "no confirm broker", http.StatusServiceUnavailable)
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

	ctx := r.Context()
	notify := s.confirmBroker.Notify()

	for {
		// Send current pending
		pending := s.confirmBroker.Pending()
		data, _ := json.Marshal(pending)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()

		// Wait for new request or disconnect
		select {
		case <-notify:
			continue
		case <-ctx.Done():
			return
		}
	}
}
