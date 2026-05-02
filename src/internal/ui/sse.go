// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"fmt"
	"net/http"
	"strings"
)

// SSEWriter writes Server-Sent Events to an HTTP response.
type SSEWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

// NewSSEWriter creates an SSE writer, setting appropriate headers.
func NewSSEWriter(w http.ResponseWriter) (*SSEWriter, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("streaming not supported")
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher.Flush()

	return &SSEWriter{w: w, flusher: flusher}, nil
}

// Event sends a named event with data.
func (sw *SSEWriter) Event(event, data string) {
	fmt.Fprintf(sw.w, "event: %s\n", event)
	for _, line := range strings.Split(data, "\n") {
		fmt.Fprintf(sw.w, "data: %s\n", line)
	}
	fmt.Fprint(sw.w, "\n")
	sw.flusher.Flush()
}
