// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"strings"
	"testing"
)

func TestRenderMarkdown_BasicFormatting(t *testing.T) {
	cases := []struct {
		name        string
		src         string
		mustContain []string
	}{
		{"empty", "", nil},
		{"whitespace only", "   \n  ", nil},
		{"plain", "hello", []string{"<p>hello</p>"}},
		{"bold + inline code", "hello **world** and `code`", []string{"<strong>world</strong>", "<code>code</code>"}},
		{"two paragraphs", "para 1\n\npara 2", []string{"<p>para 1</p>", "<p>para 2</p>"}},
		{"bullet list", "- one\n- two\n- three", []string{"<ul>", "<li>one</li>", "<li>three</li>"}},
		{"code block", "```go\nfunc Run() {}\n```", []string{"<pre>", "<code", "func Run"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := string(renderMarkdown(tc.src))
			if len(tc.mustContain) == 0 {
				if got != "" {
					t.Errorf("expected empty output, got %q", got)
				}
				return
			}
			for _, want := range tc.mustContain {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q\ngot: %s", want, got)
				}
			}
		})
	}
}

// TestRenderMarkdown_NoRawHTMLEscape guards against XSS via description
// content. goldmark's safe default replaces raw HTML with an
// "omitted" comment marker rather than rendering or escaping it; this
// test makes sure we didn't accidentally enable WithUnsafe so a
// malicious description can't inject script tags.
func TestRenderMarkdown_NoRawHTMLEscape(t *testing.T) {
	got := string(renderMarkdown(`hello <script>alert(1)</script> world`))
	if strings.Contains(got, "<script>") || strings.Contains(got, "</script>") {
		t.Errorf("raw <script> survived markdown render — possible XSS\noutput: %s", got)
	}
	// goldmark's default behavior: raw HTML blocks become
	// "<!-- raw HTML omitted -->" comments. We accept either that
	// neutralization or proper entity-encoding — both prevent XSS.
	if strings.Contains(got, "alert(1)") && strings.Contains(got, "<script") {
		t.Errorf("script tag content reachable in output\noutput: %s", got)
	}
}

func TestSplitOnBlankLineNoBreak(t *testing.T) {
	before, after, found := splitOnBlankLine("just one line")
	if found {
		t.Error("expected found=false")
	}
	if before != "just one line" {
		t.Errorf("before=%q", before)
	}
	if after != "" {
		t.Errorf("after=%q", after)
	}
}

func TestSplitOnBlankLineBasic(t *testing.T) {
	src := "first paragraph\n\nsecond paragraph\nstill second"
	before, after, found := splitOnBlankLine(src)
	if !found {
		t.Fatal("expected found=true")
	}
	if before != "first paragraph" {
		t.Errorf("before=%q", before)
	}
	if after != "second paragraph\nstill second" {
		t.Errorf("after=%q", after)
	}
}

func TestSplitOnBlankLineCRLF(t *testing.T) {
	src := "first\r\n\r\nsecond"
	before, after, _ := splitOnBlankLine(src)
	if before != "first" || after != "second" {
		t.Errorf("CRLF: before=%q after=%q", before, after)
	}
}

func TestMarkdownLeadEmpty(t *testing.T) {
	got := markdownLead("")
	if got.Lead != "" || got.Rest != "" {
		t.Errorf("expected empty MarkdownLead, got %+v", got)
	}
}

func TestMarkdownLeadSingleParagraph(t *testing.T) {
	got := markdownLead("just one paragraph")
	if !strings.Contains(string(got.Lead), "just one paragraph") {
		t.Errorf("Lead missing content: %q", got.Lead)
	}
	if got.Rest != "" {
		t.Errorf("Rest should be empty for single-paragraph input, got %q", got.Rest)
	}
}

func TestMarkdownLeadMultiParagraph(t *testing.T) {
	src := "Execute a Go script that can call other registered factorly tools.\n\nThe script runs in a sandboxed go interpreter."
	got := markdownLead(src)
	if !strings.Contains(string(got.Lead), "Execute a Go script") {
		t.Errorf("Lead missing first sentence: %q", got.Lead)
	}
	if strings.Contains(string(got.Lead), "sandboxed") {
		t.Errorf("Lead leaked the rest: %q", got.Lead)
	}
	if !strings.Contains(string(got.Rest), "sandboxed go interpreter") {
		t.Errorf("Rest missing second paragraph: %q", got.Rest)
	}
}
