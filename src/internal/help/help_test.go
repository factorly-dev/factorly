// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package help

import (
	"strings"
	"testing"

	"github.com/factorly-dev/factorly/internal/config"
)

// TestRender_Default returns the overview + behaviors block and the
// topic discovery list, even when cfg is nil. This is the worst-case
// "fresh install, no config" path — must still render usefully.
func TestRender_Default(t *testing.T) {
	out := Render(TopicDefault, Inputs{})
	for _, want := range []string{
		"# What factorly is",
		"## Behaviors to expect",
		"Credentials are already wired up",
		"shadow.confirm: true",
		"## Topics for deeper detail",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("default render missing %q", want)
		}
	}
}

// TestRender_Topics each fixed topic returns its own section without
// dragging in the full default view.
func TestRender_Topics(t *testing.T) {
	cases := map[Topic]string{
		TopicVault:      "# The vault model",
		TopicShadow:     "# Oversight (shadow rules)",
		TopicWorkflows:  "# Workflows",
		TopicBlueprints: "# Blueprints",
		TopicTools:      "# Tools available here",
		TopicWhatIs:     "# What factorly is",
	}
	for topic, want := range cases {
		t.Run(string(topic), func(t *testing.T) {
			out := Render(topic, Inputs{})
			if !strings.Contains(out, want) {
				t.Errorf("topic %q missing %q; got:\n%s", topic, want, out)
			}
		})
	}
}

// TestRender_UnknownTopic falls back to the default view with a clear
// hint about the unknown topic, instead of returning empty.
func TestRender_UnknownTopic(t *testing.T) {
	out := Render(Topic("bogus"), Inputs{})
	if !strings.Contains(out, `Unknown topic "bogus"`) {
		t.Errorf("unknown topic should be named in output; got:\n%s", out)
	}
	if !strings.Contains(out, "# What factorly is") {
		t.Errorf("unknown topic should still include the default overview; got:\n%s", out)
	}
}

// TestRender_PersonalizedSnapshot the "what's installed here" section
// reflects the cfg passed in.
func TestRender_PersonalizedSnapshot(t *testing.T) {
	cfg := &config.Config{
		Tools: map[string]config.ToolConfig{
			"github.list_repos": {Type: "rest", Description: "List repos"},
			"github.create_pr":  {Type: "rest"},
			"my.workflow":       {Type: "workflow"},
			"factorly.fetch":    {Type: "builtin"},
			"hidden.thing":      {Type: "rest", Hidden: true},
		},
	}
	out := Render(TopicDefault, Inputs{Config: cfg})
	for _, want := range []string{
		"2 user-defined tools",
		"1 workflow",          // singular — pluralization works
		"1 factorly built-in", // singular too
		"1 hidden tool",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("personalized snapshot missing %q; got:\n%s", want, out)
		}
	}
}

// TestRenderTool returns docs for a known tool and includes shadow
// info when set.
func TestRenderTool(t *testing.T) {
	cfg := &config.Config{
		Tools: map[string]config.ToolConfig{
			"demo.thing": {
				Type:        "rest",
				Description: "do a thing",
				Parameters: []config.ParamConfig{
					{Name: "url", Description: "URL to hit", Required: true},
					{Name: "method", Default: "GET"},
				},
				Shadow: &config.ShadowConfig{Confirm: true},
			},
		},
	}
	out := RenderTool("demo.thing", cfg)
	for _, want := range []string{
		"# demo.thing",
		"do a thing",
		"`url` (required)",
		"`method`",
		"default: `GET`",
		"## Oversight",
		"requires user approval",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("tool docs missing %q; got:\n%s", want, out)
		}
	}
}

// TestRenderTool_Unknown returns "" so callers can fall back.
func TestRenderTool_Unknown(t *testing.T) {
	out := RenderTool("nope", &config.Config{Tools: map[string]config.ToolConfig{}})
	if out != "" {
		t.Errorf("unknown tool should return empty string; got: %q", out)
	}
	if got := RenderTool("nope", nil); got != "" {
		t.Errorf("nil cfg should return empty string; got: %q", got)
	}
}

// TestRender_ToolsGroupingFactorlyFirst confirms the factorly.* group
// renders before user-installed tools, so an agent sees the runtime's
// own surface first.
func TestRender_ToolsGroupingFactorlyFirst(t *testing.T) {
	cfg := &config.Config{
		Tools: map[string]config.ToolConfig{
			"github.list_repos": {Type: "rest"},
			"factorly.fetch":    {Type: "builtin"},
			"aaa.bbb":           {Type: "rest"},
		},
	}
	out := Render(TopicTools, Inputs{Config: cfg})
	idxFactorly := strings.Index(out, "`factorly.*`")
	idxAaa := strings.Index(out, "`aaa.*`")
	idxGithub := strings.Index(out, "`github.*`")
	if idxFactorly < 0 || idxAaa < 0 || idxGithub < 0 {
		t.Fatalf("missing group headers; got:\n%s", out)
	}
	if idxFactorly >= idxAaa || idxFactorly >= idxGithub {
		t.Errorf("expected factorly.* first; positions f=%d a=%d g=%d", idxFactorly, idxAaa, idxGithub)
	}
}
