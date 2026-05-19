// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"bytes"
	"html/template"
	"strings"
	"sync"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/parser"
)

// markdownRenderer is the shared goldmark instance. Configured with safe
// defaults — raw HTML in input is rendered as literal text, not passed
// through, so a malicious description like "<script>alert(1)</script>"
// shows as escaped text instead of executing. Auto-link disabled to
// avoid surprising rewrites in code samples.
var (
	markdownOnce     sync.Once
	markdownInstance goldmark.Markdown
)

func markdownEngine() goldmark.Markdown {
	markdownOnce.Do(func() {
		markdownInstance = goldmark.New(
			goldmark.WithParserOptions(parser.WithAutoHeadingID()),
			// No goldmark.WithRendererOptions(html.WithUnsafe()) — raw
			// HTML stays escaped. No WithExtensions(extension.Linkify)
			// — auto-linking text URLs is too aggressive for code
			// samples (it would rewrite "https://" inside ```go blocks).
		)
	})
	return markdownInstance
}

// renderMarkdown converts a markdown source string to HTML safe to drop
// into a template. Whitespace-only input returns an empty string so the
// caller's `{{if ...}}` guard still works against the rendered output.
func renderMarkdown(src string) template.HTML {
	if strings.TrimSpace(src) == "" {
		return ""
	}
	var buf bytes.Buffer
	if err := markdownEngine().Convert([]byte(src), &buf); err != nil {
		// Fall back to plain escaped text so a broken renderer never
		// drops the description entirely.
		return template.HTML(template.HTMLEscapeString(src))
	}
	return template.HTML(buf.String())
}

// MarkdownLead splits a description into the first paragraph
// (everything up to the first blank line) and the remainder. Used by
// header description blocks so a long description like
// factorly.code's ~250-word reference collapses to a one-line lead
// with a "Show more" disclosure for the rest.
//
// When the input has no blank-line break, Lead is the whole string
// and Rest is empty — callers should render only Lead in that case.
type MarkdownLead struct {
	Lead template.HTML
	Rest template.HTML
}

// markdownLead splits the source on the first blank line and renders
// each half independently. Trimming preserves intentional blank lines
// inside the rest (e.g., paragraph breaks within the SDK reference).
func markdownLead(src string) MarkdownLead {
	if strings.TrimSpace(src) == "" {
		return MarkdownLead{}
	}
	lead, rest, _ := splitOnBlankLine(src)
	out := MarkdownLead{Lead: renderMarkdown(lead)}
	if strings.TrimSpace(rest) != "" {
		out.Rest = renderMarkdown(rest)
	}
	return out
}

// splitOnBlankLine returns (before, after, found) where `before` is
// everything up to the first blank line and `after` is everything
// after it. A blank line is a \n followed by \n (with optional
// whitespace between).
func splitOnBlankLine(s string) (string, string, bool) {
	// Normalize CRLF so we can match LF-only.
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if i == 0 {
			continue
		}
		if strings.TrimSpace(line) == "" {
			before := strings.Join(lines[:i], "\n")
			after := strings.Join(lines[i+1:], "\n")
			return before, after, true
		}
	}
	return s, "", false
}
