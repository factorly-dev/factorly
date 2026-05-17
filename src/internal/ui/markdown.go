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
