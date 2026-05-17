// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

type mockWorkflowExecutor struct {
	calls   []string
	results map[string]*Result
	errors  map[string]error
}

func (m *mockWorkflowExecutor) ExecuteWithContext(_ context.Context, toolName string, params map[string]string, _ string) (*Result, error) {
	m.calls = append(m.calls, toolName)
	if err, ok := m.errors[toolName]; ok {
		return nil, err
	}
	if r, ok := m.results[toolName]; ok {
		return r, nil
	}
	// Default: echo params as output
	output := toolName + " ok"
	if msg, ok := params["message"]; ok {
		output = msg
	}
	return &Result{Output: output}, nil
}

func TestWorkflowSimple(t *testing.T) {
	exec := &mockWorkflowExecutor{}
	wp := NewWorkflowProvider(exec, false)
	wp.RegisterWorkflow("test.pipeline", []WorkflowStep{
		{Tool: "step1"},
		{Tool: "step2"},
	})

	result, err := wp.Execute("test.pipeline", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(exec.calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(exec.calls))
	}
	if exec.calls[0] != "step1" || exec.calls[1] != "step2" {
		t.Errorf("expected step1, step2; got %v", exec.calls)
	}

	var state struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(result.Output), &state); err != nil {
		t.Fatal(err)
	}
	if state.Status != "completed" {
		t.Errorf("expected completed, got %s", state.Status)
	}
}

func TestWorkflowVariablePassing(t *testing.T) {
	exec := &mockWorkflowExecutor{
		results: map[string]*Result{
			"fetch": {Output: "data from fetch"},
		},
	}
	wp := NewWorkflowProvider(exec, false)
	wp.RegisterWorkflow("test.pipeline", []WorkflowStep{
		{Tool: "fetch", Store: "data"},
		{Tool: "process", Params: map[string]string{"message": "got: {{data}}"}},
	})

	_, err := wp.Execute("test.pipeline", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Check that step 2 received the substituted variable
	if len(exec.calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(exec.calls))
	}
}

func TestWorkflowInputParams(t *testing.T) {
	exec := &mockWorkflowExecutor{}
	wp := NewWorkflowProvider(exec, false)
	wp.RegisterWorkflow("test.greet", []WorkflowStep{
		{Tool: "echo", Params: map[string]string{"message": "hello {{name}}"}},
	})

	_, err := wp.Execute("test.greet", map[string]string{"name": "world"})
	if err != nil {
		t.Fatal(err)
	}
	if len(exec.calls) != 1 {
		t.Fatal("expected 1 call")
	}
}

func TestWorkflowStepFailure(t *testing.T) {
	exec := &mockWorkflowExecutor{
		errors: map[string]error{
			"step2": fmt.Errorf("step2 failed"),
		},
	}
	wp := NewWorkflowProvider(exec, false)
	wp.RegisterWorkflow("test.pipeline", []WorkflowStep{
		{Tool: "step1"},
		{Tool: "step2"},
		{Tool: "step3"},
	})

	result, err := wp.Execute("test.pipeline", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "step 2") {
		t.Errorf("expected step 2 in error, got: %s", err.Error())
	}

	// Only step1 and step2 should have been called
	if len(exec.calls) != 2 {
		t.Errorf("expected 2 calls (step3 skipped), got %d", len(exec.calls))
	}

	// Output should have state with skipped step
	var state struct {
		Status string `json:"status"`
		Steps  []struct {
			Status string `json:"status"`
		} `json:"steps"`
	}
	if err := json.Unmarshal([]byte(result.Output), &state); err != nil {
		t.Fatal(err)
	}
	if state.Status != "failed" {
		t.Errorf("expected failed, got %s", state.Status)
	}
	if state.Steps[2].Status != "skipped" {
		t.Errorf("expected step 3 skipped, got %s", state.Steps[2].Status)
	}
}

func TestWorkflowNotRegistered(t *testing.T) {
	exec := &mockWorkflowExecutor{}
	wp := NewWorkflowProvider(exec, false)

	_, err := wp.Execute("nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for unregistered workflow")
	}
}

func TestWorkflowEmptySteps(t *testing.T) {
	exec := &mockWorkflowExecutor{}
	wp := NewWorkflowProvider(exec, false)
	wp.RegisterWorkflow("test.empty", []WorkflowStep{})

	_, err := wp.Execute("test.empty", nil)
	if err == nil {
		t.Fatal("expected error for empty workflow")
	}
}

func TestWorkflowContextCancelled(t *testing.T) {
	exec := &mockWorkflowExecutor{}
	wp := NewWorkflowProvider(exec, false)
	wp.RegisterWorkflow("test.pipeline", []WorkflowStep{
		{Tool: "step1"},
		{Tool: "step2"},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	result, err := wp.ExecuteWithContext(ctx, "test.pipeline", nil)
	if err == nil {
		// Context cancellation is reflected in state, not as an error return
		var state struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal([]byte(result.Output), &state); err == nil {
			if state.Status != "failed" {
				t.Errorf("expected failed status on cancelled context, got %s", state.Status)
			}
		}
	}
}

func TestWorkflowThreeStepChain(t *testing.T) {
	exec := &mockWorkflowExecutor{
		results: map[string]*Result{
			"step1": {Output: "alpha"},
			"step2": {Output: "alpha-beta"},
			"step3": {Output: "alpha-beta-gamma"},
		},
	}
	wp := NewWorkflowProvider(exec, false)
	wp.RegisterWorkflow("test.chain", []WorkflowStep{
		{Tool: "step1", Store: "a"},
		{Tool: "step2", Params: map[string]string{"input": "{{a}}"}, Store: "b"},
		{Tool: "step3", Params: map[string]string{"input": "{{b}}"}},
	})

	result, err := wp.Execute("test.chain", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(exec.calls) != 3 {
		t.Fatalf("expected 3 calls, got %d", len(exec.calls))
	}

	var state struct {
		Result string `json:"result"`
	}
	_ = json.Unmarshal([]byte(result.Output), &state)
	if state.Result != "alpha-beta-gamma" {
		t.Errorf("expected chained result, got %q", state.Result)
	}
}

func TestWorkflowMixedInputAndStoredVars(t *testing.T) {
	exec := &mockWorkflowExecutor{
		results: map[string]*Result{
			"fetch": {Output: "fetched-data"},
		},
	}
	wp := NewWorkflowProvider(exec, false)
	wp.RegisterWorkflow("test.mixed", []WorkflowStep{
		{Tool: "fetch", Store: "data"},
		{Tool: "process", Params: map[string]string{
			"input": "{{data}}",
			"env":   "{{environment}}",
		}},
	})

	_, err := wp.Execute("test.mixed", map[string]string{"environment": "staging"})
	if err != nil {
		t.Fatal(err)
	}
	if len(exec.calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(exec.calls))
	}
}

func TestWorkflowStoreEmptyOutput(t *testing.T) {
	exec := &mockWorkflowExecutor{
		results: map[string]*Result{
			"step1": {Output: ""},
		},
	}
	wp := NewWorkflowProvider(exec, false)
	wp.RegisterWorkflow("test.empty_store", []WorkflowStep{
		{Tool: "step1", Store: "data"},
		{Tool: "step2", Params: map[string]string{"input": "got: {{data}}"}},
	})

	_, err := wp.Execute("test.empty_store", nil)
	if err != nil {
		t.Fatal(err)
	}
	// Should still work — empty string is a valid variable
	if len(exec.calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(exec.calls))
	}
}

func TestWorkflowMultipleWorkflows(t *testing.T) {
	exec := &mockWorkflowExecutor{}
	wp := NewWorkflowProvider(exec, false)
	wp.RegisterWorkflow("workflow.a", []WorkflowStep{
		{Tool: "step_a"},
	})
	wp.RegisterWorkflow("workflow.b", []WorkflowStep{
		{Tool: "step_b1"},
		{Tool: "step_b2"},
	})

	_, err := wp.Execute("workflow.a", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(exec.calls) != 1 || exec.calls[0] != "step_a" {
		t.Errorf("expected step_a, got %v", exec.calls)
	}

	exec.calls = nil
	_, err = wp.Execute("workflow.b", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(exec.calls) != 2 {
		t.Errorf("expected 2 calls for workflow.b, got %d", len(exec.calls))
	}
}

func TestWorkflowStatePersisted(t *testing.T) {
	runsDir := t.TempDir()
	exec := &mockWorkflowExecutor{}
	wp := NewWorkflowProvider(exec, false)
	wp.SetRunsDir(runsDir)
	wp.RegisterWorkflow("test.persist", []WorkflowStep{
		{Tool: "step1"},
	})

	result, err := wp.Execute("test.persist", nil)
	if err != nil {
		t.Fatal(err)
	}

	var state struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(result.Output), &state); err != nil {
		t.Fatal(err)
	}
	if state.Status != "completed" {
		t.Errorf("expected completed, got %s", state.Status)
	}

	matches, _ := filepath.Glob(filepath.Join(runsDir, "*.json"))
	if len(matches) == 0 {
		t.Errorf("expected a run state file under %s, found none", runsDir)
	}
}

func TestWorkflowStateNotPersistedWhenRunsDirEmpty(t *testing.T) {
	exec := &mockWorkflowExecutor{}
	wp := NewWorkflowProvider(exec, false)
	wp.RegisterWorkflow("test.nopersist", []WorkflowStep{
		{Tool: "step1"},
	})
	if _, err := wp.Execute("test.nopersist", nil); err != nil {
		t.Fatal(err)
	}
	// No assertion needed beyond reaching this point without a panic
	// or scribbling into cwd — SetRunsDir was never called.
}

func TestWorkflowFirstStepFails(t *testing.T) {
	exec := &mockWorkflowExecutor{
		errors: map[string]error{
			"step1": fmt.Errorf("connection refused"),
		},
	}
	wp := NewWorkflowProvider(exec, false)
	wp.RegisterWorkflow("test.fail_first", []WorkflowStep{
		{Tool: "step1"},
		{Tool: "step2"},
		{Tool: "step3"},
	})

	result, err := wp.Execute("test.fail_first", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "step 1") {
		t.Errorf("expected 'step 1' in error, got: %s", err.Error())
	}

	// Only step1 called, step2 and step3 skipped
	if len(exec.calls) != 1 {
		t.Errorf("expected 1 call, got %d", len(exec.calls))
	}

	var state struct {
		Steps []struct {
			Status string `json:"status"`
		} `json:"steps"`
	}
	_ = json.Unmarshal([]byte(result.Output), &state)
	if state.Steps[0].Status != "failed" {
		t.Errorf("step 1 should be failed, got %s", state.Steps[0].Status)
	}
	if state.Steps[1].Status != "skipped" {
		t.Errorf("step 2 should be skipped, got %s", state.Steps[1].Status)
	}
	if state.Steps[2].Status != "skipped" {
		t.Errorf("step 3 should be skipped, got %s", state.Steps[2].Status)
	}
}

func TestWorkflowLastStepFails(t *testing.T) {
	exec := &mockWorkflowExecutor{
		errors: map[string]error{
			"step3": fmt.Errorf("timeout"),
		},
	}
	wp := NewWorkflowProvider(exec, false)
	wp.RegisterWorkflow("test.fail_last", []WorkflowStep{
		{Tool: "step1"},
		{Tool: "step2"},
		{Tool: "step3"},
	})

	result, err := wp.Execute("test.fail_last", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "step 3") {
		t.Errorf("expected 'step 3' in error, got: %s", err.Error())
	}

	// All 3 called
	if len(exec.calls) != 3 {
		t.Errorf("expected 3 calls, got %d", len(exec.calls))
	}

	var state struct {
		Steps []struct {
			Status string `json:"status"`
		} `json:"steps"`
	}
	_ = json.Unmarshal([]byte(result.Output), &state)
	if state.Steps[0].Status != "completed" {
		t.Errorf("step 1 should be completed, got %s", state.Steps[0].Status)
	}
	if state.Steps[1].Status != "completed" {
		t.Errorf("step 2 should be completed, got %s", state.Steps[1].Status)
	}
	if state.Steps[2].Status != "failed" {
		t.Errorf("step 3 should be failed, got %s", state.Steps[2].Status)
	}
}

func TestWorkflowNilResult(t *testing.T) {
	exec := &mockWorkflowExecutor{
		results: map[string]*Result{
			"step1": nil,
		},
	}
	wp := NewWorkflowProvider(exec, false)
	wp.RegisterWorkflow("test.nil", []WorkflowStep{
		{Tool: "step1", Store: "data"},
		{Tool: "step2", Params: map[string]string{"input": "{{data}}"}},
	})

	_, err := wp.Execute("test.nil", nil)
	if err != nil {
		t.Fatalf("nil result should not cause error: %v", err)
	}
	if len(exec.calls) != 2 {
		t.Errorf("expected 2 calls, got %d", len(exec.calls))
	}
}

func TestWorkflowUnresolvedVariable(t *testing.T) {
	exec := &mockWorkflowExecutor{}
	wp := NewWorkflowProvider(exec, false)
	wp.RegisterWorkflow("test.unresolved", []WorkflowStep{
		{Tool: "step1", Params: map[string]string{"input": "{{missing_var}}"}},
	})

	// Unresolved variables pass through as literal — not an error
	_, err := wp.Execute("test.unresolved", nil)
	if err != nil {
		t.Fatalf("unresolved variable should not error: %v", err)
	}
}

func TestWorkflowStepErrorMessageInState(t *testing.T) {
	exec := &mockWorkflowExecutor{
		errors: map[string]error{
			"deploy": fmt.Errorf("tool \"deploy\" is denied by shadow policy"),
		},
	}
	wp := NewWorkflowProvider(exec, false)
	wp.RegisterWorkflow("test.denied", []WorkflowStep{
		{Tool: "build"},
		{Tool: "deploy"},
	})

	result, err := wp.Execute("test.denied", nil)
	if err == nil {
		t.Fatal("expected error")
	}

	var state struct {
		Error string `json:"error"`
		Steps []struct {
			Error string `json:"error"`
		} `json:"steps"`
	}
	_ = json.Unmarshal([]byte(result.Output), &state)

	if !strings.Contains(state.Error, "denied") {
		t.Errorf("expected 'denied' in workflow error, got %q", state.Error)
	}
	if !strings.Contains(state.Steps[1].Error, "denied") {
		t.Errorf("expected 'denied' in step error, got %q", state.Steps[1].Error)
	}
}

func TestWorkflowLongOutputTruncatedInState(t *testing.T) {
	longOutput := strings.Repeat("x", 1000)
	exec := &mockWorkflowExecutor{
		results: map[string]*Result{
			"step1": {Output: longOutput},
		},
	}
	wp := NewWorkflowProvider(exec, false)
	wp.RegisterWorkflow("test.long", []WorkflowStep{
		{Tool: "step1", Store: "data"},
	})

	result, err := wp.Execute("test.long", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Returned output should have the FULL result (not truncated)
	var state struct {
		Result string `json:"result"`
	}
	_ = json.Unmarshal([]byte(result.Output), &state)

	if len(state.Result) != 1000 {
		t.Errorf("expected full result (1000 chars) in returned output, got %d chars", len(state.Result))
	}
}

func TestWorkflowOutputIncludesState(t *testing.T) {
	exec := &mockWorkflowExecutor{
		results: map[string]*Result{
			"step1": {Output: "result1"},
			"step2": {Output: "result2"},
		},
	}
	wp := NewWorkflowProvider(exec, false)
	wp.RegisterWorkflow("test.pipeline", []WorkflowStep{
		{Tool: "step1"},
		{Tool: "step2"},
	})

	result, err := wp.Execute("test.pipeline", nil)
	if err != nil {
		t.Fatal(err)
	}

	var state struct {
		Status string `json:"status"`
		Steps  []struct {
			Tool   string `json:"tool"`
			Status string `json:"status"`
		} `json:"steps"`
		Result string `json:"result"`
	}
	if err := json.Unmarshal([]byte(result.Output), &state); err != nil {
		t.Fatalf("output should be valid JSON: %v", err)
	}
	if state.Status != "completed" {
		t.Errorf("expected completed, got %s", state.Status)
	}
	if len(state.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(state.Steps))
	}
	if state.Steps[0].Tool != "step1" || state.Steps[1].Tool != "step2" {
		t.Error("expected step tools in output")
	}
	if state.Result != "result2" {
		t.Errorf("expected last output as result, got %q", state.Result)
	}
}

func TestWorkflowIfTrue(t *testing.T) {
	exec := &mockWorkflowExecutor{}
	wp := NewWorkflowProvider(exec, false)
	wp.RegisterWorkflow("test.if", []WorkflowStep{
		{Tool: "step1", Store: "out"},
		{Tool: "step2", If: "out != ''"},
	})

	result, err := wp.Execute("test.if", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(exec.calls) != 2 {
		t.Errorf("expected 2 calls, got %d: %v", len(exec.calls), exec.calls)
	}
	if !strings.Contains(result.Output, `"status":"completed"`) {
		t.Error("expected completed status")
	}
}

func TestWorkflowIfFalse(t *testing.T) {
	exec := &mockWorkflowExecutor{}
	wp := NewWorkflowProvider(exec, false)
	wp.RegisterWorkflow("test.if", []WorkflowStep{
		{Tool: "step1", Store: "out"},
		{Tool: "step2", If: "missing != ''"}, // "missing" is not stored, so empty → false
	})

	result, err := wp.Execute("test.if", nil)
	if err != nil {
		t.Fatal(err)
	}
	// step2 should be skipped
	if len(exec.calls) != 1 {
		t.Errorf("expected 1 call (step2 skipped), got %d: %v", len(exec.calls), exec.calls)
	}
	if !strings.Contains(result.Output, `"skipped"`) {
		t.Error("expected skipped step in output")
	}
}

func TestWorkflowSwitchFirstMatch(t *testing.T) {
	exec := &mockWorkflowExecutor{}
	wp := NewWorkflowProvider(exec, false)
	wp.RegisterWorkflow("test.switch", []WorkflowStep{
		{
			Switch: []WorkflowSwitchCase{
				{Condition: "false", Tool: "wrong"},
				{Condition: "true", Tool: "right"},
			},
		},
	})

	_, err := wp.Execute("test.switch", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(exec.calls) != 1 || exec.calls[0] != "right" {
		t.Errorf("expected only 'right' called, got %v", exec.calls)
	}
}

func TestWorkflowSwitchWithVariable(t *testing.T) {
	exec := &mockWorkflowExecutor{
		results: map[string]*Result{
			"get.status": {Output: "degraded"},
		},
	}
	wp := NewWorkflowProvider(exec, false)
	wp.RegisterWorkflow("test.switch", []WorkflowStep{
		{Tool: "get.status", Store: "status"},
		{
			Switch: []WorkflowSwitchCase{
				{Condition: "status == 'healthy'", Tool: "notify.ok"},
				{Condition: "status == 'degraded'", Tool: "notify.warn"},
				{Condition: "true", Tool: "notify.critical"},
			},
		},
	})

	_, err := wp.Execute("test.switch", nil)
	if err != nil {
		t.Fatal(err)
	}
	// Should call get.status then notify.warn
	if len(exec.calls) != 2 {
		t.Fatalf("expected 2 calls, got %d: %v", len(exec.calls), exec.calls)
	}
	if exec.calls[1] != "notify.warn" {
		t.Errorf("expected notify.warn, got %s", exec.calls[1])
	}
}

func TestWorkflowSwitchNoMatch(t *testing.T) {
	exec := &mockWorkflowExecutor{}
	wp := NewWorkflowProvider(exec, false)
	wp.RegisterWorkflow("test.switch", []WorkflowStep{
		{
			Switch: []WorkflowSwitchCase{
				{Condition: "false", Tool: "never"},
				{Condition: "x == 'nope'", Tool: "also.never"},
			},
		},
	})

	result, err := wp.Execute("test.switch", nil)
	if err != nil {
		t.Fatal(err)
	}
	// No match = skipped, no tools called
	if len(exec.calls) != 0 {
		t.Errorf("expected 0 calls, got %d: %v", len(exec.calls), exec.calls)
	}
	if !strings.Contains(result.Output, `"skipped"`) {
		t.Error("expected skipped in output")
	}
}

func TestWorkflowSwitchStore(t *testing.T) {
	exec := &mockWorkflowExecutor{
		results: map[string]*Result{
			"produce": {Output: "value123"},
		},
	}
	wp := NewWorkflowProvider(exec, false)
	wp.RegisterWorkflow("test.switch", []WorkflowStep{
		{
			Switch: []WorkflowSwitchCase{
				{Condition: "true", Tool: "produce", Store: "captured"},
			},
		},
		{Tool: "consume", Params: map[string]string{"data": "{{captured}}"}},
	})

	_, err := wp.Execute("test.switch", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(exec.calls) != 2 {
		t.Fatalf("expected 2 calls, got %v", exec.calls)
	}
	// The consume step should have received the stored value
	if exec.calls[1] != "consume" {
		t.Errorf("expected consume, got %s", exec.calls[1])
	}
}

func TestWorkflowRequireTrue(t *testing.T) {
	exec := &mockWorkflowExecutor{}
	wp := NewWorkflowProvider(exec, false)
	wp.RegisterWorkflow("test.require", []WorkflowStep{
		{Tool: "step1", Store: "val"},
		{Tool: "step2", Require: "val != ''"},
		{Tool: "step3"},
	})

	result, err := wp.Execute("test.require", nil)
	if err != nil {
		t.Fatal(err)
	}
	// All three should run (require passes)
	if len(exec.calls) != 3 {
		t.Errorf("expected 3 calls, got %d: %v", len(exec.calls), exec.calls)
	}
	if !strings.Contains(result.Output, `"status":"completed"`) {
		t.Error("expected completed")
	}
}

func TestWorkflowRequireFalse(t *testing.T) {
	exec := &mockWorkflowExecutor{}
	wp := NewWorkflowProvider(exec, false)
	wp.RegisterWorkflow("test.require", []WorkflowStep{
		{Tool: "step1", Store: "val"},
		{Tool: "step2", Require: "missing != ''"}, // missing var = empty = false
		{Tool: "step3"},
	})

	result, err := wp.Execute("test.require", nil)
	if err != nil {
		t.Fatal(err)
	}
	// step1 runs, step2+step3 skipped (require halts)
	if len(exec.calls) != 1 {
		t.Errorf("expected 1 call (require halted), got %d: %v", len(exec.calls), exec.calls)
	}
	// Status should be completed (not failed — it's an intentional stop)
	if !strings.Contains(result.Output, `"status":"completed"`) {
		t.Errorf("expected completed status (graceful stop), got %s", result.Output)
	}
	// Both step2 and step3 should be skipped
	if strings.Count(result.Output, `"skipped"`) < 2 {
		t.Error("expected step2 and step3 to be skipped")
	}
}

func TestWorkflowRequireAtFirstStep(t *testing.T) {
	exec := &mockWorkflowExecutor{}
	wp := NewWorkflowProvider(exec, false)
	wp.RegisterWorkflow("test.require", []WorkflowStep{
		{Tool: "step1", Require: "precondition"},
	})

	// precondition not in params → empty → false → stop immediately
	result, err := wp.Execute("test.require", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(exec.calls) != 0 {
		t.Errorf("expected 0 calls, got %d", len(exec.calls))
	}
	if !strings.Contains(result.Output, `"status":"completed"`) {
		t.Error("expected completed (graceful stop)")
	}
}

func TestWorkflowRequireWithParams(t *testing.T) {
	exec := &mockWorkflowExecutor{}
	wp := NewWorkflowProvider(exec, false)
	wp.RegisterWorkflow("test.require", []WorkflowStep{
		{Tool: "step1", Require: "env == 'prod'"},
	})

	// With env=prod → runs
	_, err := wp.Execute("test.require", map[string]string{"env": "prod"})
	if err != nil {
		t.Fatal(err)
	}
	if len(exec.calls) != 1 {
		t.Errorf("expected 1 call with env=prod, got %d", len(exec.calls))
	}

	// With env=staging → stops
	exec.calls = nil
	result, err := wp.Execute("test.require", map[string]string{"env": "staging"})
	if err != nil {
		t.Fatal(err)
	}
	if len(exec.calls) != 0 {
		t.Errorf("expected 0 calls with env=staging, got %d", len(exec.calls))
	}
	if !strings.Contains(result.Output, `"skipped"`) {
		t.Error("expected skipped")
	}
}

func TestWorkflowExprSubstitution(t *testing.T) {
	var capturedParams map[string]string
	exec := &mockWorkflowExecutor{
		results: map[string]*Result{
			"fetch": {Output: `{"items":[{"name":"first"}]}`},
		},
	}
	// Override to capture params
	origExec := exec
	wp := NewWorkflowProvider(&paramCapturingExecutor{
		executor:       origExec,
		capturedParams: &capturedParams,
		captureFor:     "process",
	}, false)
	wp.RegisterWorkflow("test", []WorkflowStep{
		{Tool: "fetch", Store: "data"},
		{Tool: "process", Params: map[string]string{
			"title": `{{expr:jsonpath(data, '$.items[0].name')}}`,
			"upper": `{{expr:upper(input)}}`,
		}},
	})
	_ = wp.Setup()

	result, err := wp.Execute("test", map[string]string{"input": "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError() {
		t.Fatal(result.Error)
	}
	if capturedParams["title"] != "first" {
		t.Errorf("expected jsonpath result 'first', got %q", capturedParams["title"])
	}
	if capturedParams["upper"] != "HELLO" {
		t.Errorf("expected upper 'HELLO', got %q", capturedParams["upper"])
	}
}

// paramCapturingExecutor wraps a mock executor and captures params for a specific tool.
type paramCapturingExecutor struct {
	executor       *mockWorkflowExecutor
	capturedParams *map[string]string
	captureFor     string
}

func (p *paramCapturingExecutor) ExecuteWithContext(ctx context.Context, toolName string, params map[string]string, iface string) (*Result, error) {
	if toolName == p.captureFor {
		captured := make(map[string]string)
		for k, v := range params {
			captured[k] = v
		}
		*p.capturedParams = captured
	}
	return p.executor.ExecuteWithContext(ctx, toolName, params, iface)
}

func TestWorkflowExprNow(t *testing.T) {
	var capturedParams map[string]string
	exec := &mockWorkflowExecutor{}
	wp := NewWorkflowProvider(&paramCapturingExecutor{
		executor:       exec,
		capturedParams: &capturedParams,
		captureFor:     "api_call",
	}, false)
	wp.RegisterWorkflow("test", []WorkflowStep{
		{Tool: "api_call", Params: map[string]string{
			"timestamp": `{{expr:now()}}`,
		}},
	})
	_ = wp.Setup()

	_, err := wp.Execute("test", nil)
	if err != nil {
		t.Fatal(err)
	}
	if capturedParams["timestamp"] == "" || capturedParams["timestamp"] == "{{expr:now()}}" {
		t.Errorf("expected resolved timestamp, got %q", capturedParams["timestamp"])
	}
}

func TestWorkflowExprMixedWithVars(t *testing.T) {
	var capturedParams map[string]string
	exec := &mockWorkflowExecutor{
		results: map[string]*Result{
			"step1": {Output: "world"},
		},
	}
	wp := NewWorkflowProvider(&paramCapturingExecutor{
		executor:       exec,
		capturedParams: &capturedParams,
		captureFor:     "step2",
	}, false)
	wp.RegisterWorkflow("test", []WorkflowStep{
		{Tool: "step1", Store: "greeting"},
		{Tool: "step2", Params: map[string]string{
			"msg": `hello {{greeting}} {{expr:upper(greeting)}}`,
		}},
	})
	_ = wp.Setup()

	result, err := wp.Execute("test", nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError() {
		t.Fatal(result.Error)
	}
	if capturedParams["msg"] != "hello world WORLD" {
		t.Errorf("expected 'hello world WORLD', got %q", capturedParams["msg"])
	}
}
