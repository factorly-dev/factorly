// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/factorly-dev/factorly/internal/registry"
)

// AskRequest is one pending question waiting for a user answer in the
// browser. The shape mirrors ConfirmRequest closely — same broker
// topology — but the response is the user's typed answer (a string)
// rather than a boolean.
//
// Question-shape fields (Type, Default, Required, Enum, …) travel with
// the request so the SSE handler can serialize them and the browser
// can render the right widget without a separate lookup.
type AskRequest struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Default     string   `json:"default"`
	Required    bool     `json:"required"`
	Enum        []string `json:"enum,omitempty"`

	response chan askResponse
}

// askResponse is the internal payload on the response channel. We
// distinguish answer-with-value from a user-initiated cancel so the
// builtin handler can return distinct errors ("user cancelled" vs
// "timed out" vs the answer).
type askResponse struct {
	answer    string
	cancelled bool
}

// ErrAskCancelled is returned when the user dismisses the modal.
var ErrAskCancelled = errors.New("user cancelled the prompt")

// AskBroker manages pending question prompts between callers (the
// factorly.ask builtin) and the browser UI. Topology matches
// ConfirmBroker: per-request channel, SSE subscribers notified on
// every add/remove.
type AskBroker struct {
	mu          sync.Mutex
	pending     map[string]*AskRequest
	subscribers map[chan struct{}]struct{}
	counter     atomic.Int64
}

// NewAskBroker returns an empty broker ready to accept requests.
func NewAskBroker() *AskBroker {
	return &AskBroker{
		pending:     make(map[string]*AskRequest),
		subscribers: make(map[chan struct{}]struct{}),
	}
}

// Request posts a question and blocks until the browser responds or
// ctx is cancelled. Returns the user's answer on success,
// ErrAskCancelled if the user dismissed the modal, or the ctx error
// (typically context.DeadlineExceeded for the per-call timeout).
//
// req.ID and req.response are populated here — callers fill in the
// question-shape fields only.
func (b *AskBroker) Request(ctx context.Context, req AskRequest) (string, error) {
	req.ID = fmt.Sprintf("ask-%d", b.counter.Add(1))
	req.response = make(chan askResponse, 1)
	r := &req

	b.mu.Lock()
	b.pending[req.ID] = r
	b.notifyLocked()
	b.mu.Unlock()

	defer func() {
		b.mu.Lock()
		delete(b.pending, req.ID)
		b.notifyLocked()
		b.mu.Unlock()
	}()

	select {
	case resp := <-r.response:
		if resp.cancelled {
			return "", ErrAskCancelled
		}
		return resp.answer, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// notifyLocked wakes all SSE subscribers. Caller must hold b.mu.
func (b *AskBroker) notifyLocked() {
	for ch := range b.subscribers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// Respond resolves a pending ask with the user's answer. Returns
// false if the ID is unknown / already resolved.
func (b *AskBroker) Respond(id, answer string) bool {
	return b.deliver(id, askResponse{answer: answer})
}

// Cancel resolves a pending ask as a user-initiated cancel. The
// caller's Request returns ErrAskCancelled.
func (b *AskBroker) Cancel(id string) bool {
	return b.deliver(id, askResponse{cancelled: true})
}

func (b *AskBroker) deliver(id string, resp askResponse) bool {
	b.mu.Lock()
	req, ok := b.pending[id]
	b.mu.Unlock()
	if !ok {
		return false
	}
	select {
	case req.response <- resp:
		return true
	default:
		return false
	}
}

// Pending returns all pending asks (for SSE snapshots and the
// polling fallback).
func (b *AskBroker) Pending() []*AskRequest {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]*AskRequest, 0, len(b.pending))
	for _, r := range b.pending {
		out = append(out, r)
	}
	return out
}

// Get returns the pending request with the given ID, or nil if it's
// already resolved / unknown. Used by the POST handler to look up
// the question's constraints for server-side answer validation.
func (b *AskBroker) Get(id string) *AskRequest {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.pending[id]
}

// Subscribe returns a channel signaled on every pending-set change.
// Caller must call Unsubscribe to clean up.
func (b *AskBroker) Subscribe() chan struct{} {
	ch := make(chan struct{}, 1)
	b.mu.Lock()
	b.subscribers[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber channel.
func (b *AskBroker) Unsubscribe(ch chan struct{}) {
	b.mu.Lock()
	delete(b.subscribers, ch)
	b.mu.Unlock()
}

// --- HTTP handlers ---

func (s *Server) handleAskPending(w http.ResponseWriter, r *http.Request) {
	if s.askBroker == nil {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
		return
	}
	pending := s.askBroker.Pending()
	if pending == nil {
		pending = []*AskRequest{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(pending)
}

// handleAskRespond handles both submits and cancels. The action lives
// in path as a single endpoint with a form body; the browser POSTs
// `action=submit&answer=...` or `action=cancel`.
func (s *Server) handleAskRespond(w http.ResponseWriter, r *http.Request) {
	if s.askBroker == nil {
		http.Error(w, "no ask broker", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	req := s.askBroker.Get(id)
	if req == nil {
		http.Error(w, "ask request not found or already resolved", http.StatusNotFound)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	if r.FormValue("action") == "cancel" {
		s.askBroker.Cancel(id)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	answer := r.FormValue("answer")
	// Server-side validation against the question's declared
	// constraints. Reuses the registry's validator by synthesizing
	// a Parameter from the request so coercion (boolean "yes"/"1",
	// integer parsing, enum/pattern checks) stays consistent with
	// every other tool param in the system.
	coerced, vErrs := validateAskAnswer(req, answer)
	if len(vErrs) > 0 {
		http.Error(w, strings.Join(vErrs, "; "), http.StatusBadRequest)
		return
	}
	if !s.askBroker.Respond(id, coerced) {
		http.Error(w, "ask request not found or already resolved", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleAskSSE streams the pending-ask snapshot on every change.
// Identical wire shape to /confirm/stream.
func (s *Server) handleAskSSE(w http.ResponseWriter, r *http.Request) {
	if s.askBroker == nil {
		http.Error(w, "no ask broker", http.StatusServiceUnavailable)
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
	notify := s.askBroker.Subscribe()
	defer s.askBroker.Unsubscribe(notify)

	for {
		pending := s.askBroker.Pending()
		data, _ := json.Marshal(pending)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
		select {
		case <-notify:
			continue
		case <-ctx.Done():
			return
		}
	}
}

// validateAskAnswer runs the user's raw form value through the same
// validator other tool params use. Returns the coerced string value
// (e.g. booleans normalized to "true"/"false") and a slice of error
// strings — non-empty means the modal should stay open with the
// errors displayed inline.
//
// Required is enforced here because ValidateAndCoerce only runs rules
// on params that are PRESENT in the map; an empty string still counts
// as "present" but we want to reject it explicitly for required
// questions so the user sees a clean message.
func validateAskAnswer(req *AskRequest, raw string) (string, []string) {
	if strings.TrimSpace(raw) == "" {
		if req.Required {
			return "", []string{req.Name + ": required"}
		}
		// Empty non-required answer skips coercion / validation —
		// pass through verbatim so callers see "" rather than a
		// coerced default.
		return raw, nil
	}
	p := registry.Parameter{
		Name:        req.Name,
		Type:        req.Type,
		Required:    req.Required,
		Default:     req.Default,
		Enum:        req.Enum,
		Description: req.Description,
	}
	tool := &registry.Tool{Parameters: []registry.Parameter{p}}
	params := map[string]string{req.Name: raw}
	res := tool.ValidateAndCoerce(params)
	if res.HasErrors() {
		return "", res.Errors
	}
	return params[req.Name], nil
}
