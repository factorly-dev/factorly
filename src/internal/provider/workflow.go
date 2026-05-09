// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// WorkflowExecutor executes a tool call through the proxy.
// Defined as an interface to avoid import cycle with proxy.
type WorkflowExecutor interface {
	ExecuteWithContext(ctx context.Context, toolName string, params map[string]string, iface string) (*Result, error)
}

// WorkflowStep defines a single step in a workflow.
type WorkflowStep struct {
	Tool    string
	Params  map[string]string
	Store   string
	If      string
	Require string
	Switch  []WorkflowSwitchCase
}

type WorkflowSwitchCase struct {
	Condition string
	Tool      string
	Params    map[string]string
	Store     string
}

// WorkflowState tracks the full state of a workflow run.
type WorkflowState struct {
	WorkflowName string            `json:"workflow"`
	RunID        string            `json:"run_id"`
	Status       string            `json:"status"`
	Steps        []StepState       `json:"steps"`
	Variables    map[string]string `json:"variables"`
	StartedAt    time.Time         `json:"started_at"`
	CompletedAt  time.Time         `json:"completed_at,omitempty"`
	Error        string            `json:"error,omitempty"`
	Result       string            `json:"result,omitempty"`
}

// StepState tracks the state of a single step.
type StepState struct {
	Index      int    `json:"index"`
	Tool       string `json:"tool"`
	Status     string `json:"status"`
	DurationMs int64  `json:"duration_ms,omitempty"`
	Output     string `json:"output,omitempty"`
	Error      string `json:"error,omitempty"`
}

// WorkflowProvider executes workflow tool types.
type WorkflowProvider struct {
	executor WorkflowExecutor
	steps    map[string][]WorkflowStep
	verbose  bool
}

// NewWorkflowProvider creates a workflow provider.
func NewWorkflowProvider(exec WorkflowExecutor, verbose bool) *WorkflowProvider {
	return &WorkflowProvider{
		executor: exec,
		steps:    make(map[string][]WorkflowStep),
		verbose:  verbose,
	}
}

// RegisterWorkflow adds a workflow's steps.
func (p *WorkflowProvider) RegisterWorkflow(toolName string, steps []WorkflowStep) {
	p.steps[toolName] = steps
}

// RemoveWorkflow removes a workflow's steps.
func (p *WorkflowProvider) RemoveWorkflow(toolName string) {
	delete(p.steps, toolName)
}

func (p *WorkflowProvider) Setup() error    { return nil }
func (p *WorkflowProvider) Teardown() error { return nil }

// Execute runs a workflow's steps sequentially.
func (p *WorkflowProvider) Execute(toolName string, params map[string]string) (*Result, error) {
	return p.ExecuteWithContext(context.Background(), toolName, params)
}

// ExecuteWithContext runs a workflow with context for cancellation/timeout.
func (p *WorkflowProvider) ExecuteWithContext(ctx context.Context, toolName string, params map[string]string) (*Result, error) {
	steps, ok := p.steps[toolName]
	if !ok {
		return nil, fmt.Errorf("workflow %q not registered", toolName)
	}
	if len(steps) == 0 {
		return nil, fmt.Errorf("workflow %q has no steps", toolName)
	}

	runID := uuid.New().String()[:8]
	state := &WorkflowState{
		WorkflowName: toolName,
		RunID:        runID,
		Status:       "running",
		Variables:    make(map[string]string),
		StartedAt:    time.Now(),
	}

	// Initialize variables from input params
	for k, v := range params {
		state.Variables[k] = v
	}

	// Initialize step states
	for i, step := range steps {
		state.Steps = append(state.Steps, StepState{
			Index:  i + 1,
			Tool:   step.Tool,
			Status: "pending",
		})
	}

	if p.verbose {
		fmt.Fprintf(os.Stderr, "[workflow] %s started (run: %s)\n", toolName, runID)
	}

	var lastOutput string

	for i, step := range steps {
		// Check context
		if ctx.Err() != nil {
			state.Status = "failed"
			state.Error = "context cancelled"
			for j := i; j < len(steps); j++ {
				state.Steps[j].Status = "skipped"
			}
			break
		}

		// Require: stop workflow if condition is false
		if step.Require != "" && !EvalCondition(step.Require, state.Variables) {
			state.Steps[i].Status = "skipped"
			state.Status = "completed"
			state.CompletedAt = time.Now()
			// Mark remaining steps as skipped
			for j := i + 1; j < len(steps); j++ {
				state.Steps[j].Status = "skipped"
			}
			if p.verbose {
				fmt.Fprintf(os.Stderr, "[workflow]   %d/%d %-25s stopped    (require: %s)\n", i+1, len(steps), step.Tool, step.Require)
			}
			state.Result = truncateForState(lastOutput)
			p.saveState(state)
			state.Result = lastOutput
			return &Result{Output: marshalState(state)}, nil
		}

		// Conditional: skip step if `if` condition is false
		if step.If != "" && !EvalCondition(step.If, state.Variables) {
			state.Steps[i].Status = "skipped"
			if p.verbose {
				fmt.Fprintf(os.Stderr, "[workflow]   %d/%d %-25s skipped    (if: %s)\n", i+1, len(steps), step.Tool, step.If)
			}
			continue
		}

		// Switch step: evaluate cases, execute first match
		if len(step.Switch) > 0 {
			matched := false
			for _, c := range step.Switch {
				if EvalCondition(c.Condition, state.Variables) {
					matched = true
					state.Steps[i].Status = "running"
					state.Steps[i].Tool = c.Tool

					if p.verbose {
						fmt.Fprintf(os.Stderr, "[workflow]   %d/%d %-25s running... (switch → %s)\n", i+1, len(steps), c.Tool, c.Condition)
					}

					resolvedParams := make(map[string]string, len(c.Params))
					for k, v := range c.Params {
						resolvedParams[k] = substituteVars(v, state.Variables)
					}

					stepStart := time.Now()
					result, err := p.executor.ExecuteWithContext(ctx, c.Tool, resolvedParams, "workflow")
					duration := time.Since(stepStart)
					state.Steps[i].DurationMs = duration.Milliseconds()

					if err != nil {
						state.Steps[i].Status = "failed"
						state.Steps[i].Error = err.Error()
						state.Status = "failed"
						state.Error = fmt.Sprintf("step %d switch (%s): %s", i+1, c.Tool, err)
						for j := i + 1; j < len(steps); j++ {
							state.Steps[j].Status = "skipped"
						}
						p.saveState(state)
						return &Result{Output: marshalState(state), Error: state.Error},
							fmt.Errorf("workflow %q failed at step %d switch (%s): %w", toolName, i+1, c.Tool, err)
					}

					output := ""
					if result != nil {
						output = result.Output
					}
					state.Steps[i].Status = "completed"
					state.Steps[i].Output = truncateForState(output)
					lastOutput = output
					if c.Store != "" {
						state.Variables[c.Store] = output
					}

					if p.verbose {
						fmt.Fprintf(os.Stderr, "[workflow]   %d/%d %-25s completed  %s\n", i+1, len(steps), c.Tool, duration.Truncate(time.Millisecond))
					}
					p.saveState(state)
					break
				}
			}
			if !matched {
				state.Steps[i].Status = "skipped"
				if p.verbose {
					var conds []string
					for _, c := range step.Switch {
						conds = append(conds, c.Condition)
					}
					fmt.Fprintf(os.Stderr, "[workflow]   %d/%d %-25s skipped    (no match: %s)\n", i+1, len(steps), "switch", strings.Join(conds, "; "))
				}
			}
			continue
		}

		state.Steps[i].Status = "running"

		if p.verbose {
			fmt.Fprintf(os.Stderr, "[workflow]   %d/%d %-25s running...\n", i+1, len(steps), step.Tool)
		}

		// Substitute variables in step params
		resolvedParams := make(map[string]string, len(step.Params))
		for k, v := range step.Params {
			resolvedParams[k] = substituteVars(v, state.Variables)
		}

		stepStart := time.Now()
		result, err := p.executor.ExecuteWithContext(ctx, step.Tool, resolvedParams, "workflow")
		duration := time.Since(stepStart)

		state.Steps[i].DurationMs = duration.Milliseconds()

		if err != nil {
			state.Steps[i].Status = "failed"
			state.Steps[i].Error = err.Error()
			state.Status = "failed"
			state.Error = fmt.Sprintf("step %d (%s): %s", i+1, step.Tool, err)

			// Mark remaining steps as skipped
			for j := i + 1; j < len(steps); j++ {
				state.Steps[j].Status = "skipped"
			}

			if p.verbose {
				fmt.Fprintf(os.Stderr, "[workflow]   %d/%d %-25s failed     %s\n", i+1, len(steps), step.Tool, err)
			}

			p.saveState(state)
			return &Result{
				Output: marshalState(state),
				Error:  state.Error,
			}, fmt.Errorf("workflow %q failed at step %d (%s): %w", toolName, i+1, step.Tool, err)
		}

		output := ""
		if result != nil {
			output = result.Output
		}

		state.Steps[i].Status = "completed"
		state.Steps[i].Output = truncateForState(output)
		lastOutput = output

		// Store output as variable
		if step.Store != "" {
			state.Variables[step.Store] = output
		}

		if p.verbose {
			fmt.Fprintf(os.Stderr, "[workflow]   %d/%d %-25s completed  %s\n", i+1, len(steps), step.Tool, duration.Truncate(time.Millisecond))
		}

		p.saveState(state)
	}

	if state.Status == "running" {
		state.Status = "completed"
	}
	state.CompletedAt = time.Now()

	if p.verbose {
		totalDuration := time.Since(state.StartedAt).Truncate(time.Millisecond)
		fmt.Fprintf(os.Stderr, "[workflow] %s %s (%d steps, %s)\n", toolName, state.Status, len(steps), totalDuration)
	}

	// Save state with truncated result (keep file small)
	state.Result = truncateForState(lastOutput)
	p.saveState(state)

	// Return full result to caller (not truncated)
	state.Result = lastOutput
	return &Result{
		Output: marshalState(state),
	}, nil
}

var exprPattern = regexp.MustCompile(`\{\{expr:(.+?)\}\}`)

func substituteVars(tmpl string, vars map[string]string) string {
	// First: evaluate {{expr:...}} patterns
	result := exprPattern.ReplaceAllStringFunc(tmpl, func(match string) string {
		expr := match[7 : len(match)-2] // strip {{expr: and }}
		return EvalExpr(expr, vars)
	})
	// Then: simple {{var}} substitution (backward compatible)
	for k, v := range vars {
		result = strings.ReplaceAll(result, "{{"+k+"}}", v)
	}
	return result
}

func marshalState(state *WorkflowState) string {
	// Return a compact summary for the tool output
	type stepSummary struct {
		Tool       string `json:"tool"`
		Status     string `json:"status"`
		DurationMs int64  `json:"duration_ms,omitempty"`
		Error      string `json:"error,omitempty"`
	}
	summary := struct {
		Status string        `json:"status"`
		Steps  []stepSummary `json:"steps"`
		Result string        `json:"result,omitempty"`
		Error  string        `json:"error,omitempty"`
	}{
		Status: state.Status,
		Result: state.Result,
		Error:  state.Error,
	}
	for _, s := range state.Steps {
		summary.Steps = append(summary.Steps, stepSummary{
			Tool:       s.Tool,
			Status:     s.Status,
			DurationMs: s.DurationMs,
			Error:      s.Error,
		})
	}
	data, _ := json.Marshal(summary)
	return string(data)
}

func truncateForState(s string) string {
	if len(s) > 500 {
		return s[:500] + "..."
	}
	return s
}

func (p *WorkflowProvider) saveState(state *WorkflowState) {
	dir := filepath.Join(".factorly", "workflows")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return
	}

	tmp := filepath.Join(dir, state.RunID+".json.tmp")
	path := filepath.Join(dir, state.RunID+".json")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return
	}
	_ = os.Rename(tmp, path)
}
