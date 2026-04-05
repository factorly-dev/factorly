package provider

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

type CLIToolDef struct {
	Command string
	Args    []string
	Stdin   string // template with {param} placeholders, piped to subprocess stdin
	Env     map[string]string
	Timeout time.Duration
}

type CLIProvider struct {
	tools map[string]CLIToolDef
}

func NewCLI(tools map[string]CLIToolDef) *CLIProvider {
	return &CLIProvider{tools: tools}
}

func (p *CLIProvider) Setup() error {
	return nil
}

func (p *CLIProvider) Teardown() error {
	return nil
}

func (p *CLIProvider) Execute(toolName string, params map[string]string) (*Result, error) {
	def, ok := p.tools[toolName]
	if !ok {
		return nil, fmt.Errorf("cli provider: tool %q not registered", toolName)
	}

	args := substituteArgs(def.Args, params)

	// Check for unresolved placeholders
	for _, arg := range args {
		if idx := strings.Index(arg, "{"); idx != -1 {
			end := strings.Index(arg[idx:], "}")
			if end != -1 {
				placeholder := arg[idx+1 : idx+end]
				return nil, fmt.Errorf("cli provider: unresolved parameter {%s} in tool %q", placeholder, toolName)
			}
		}
	}

	timeout := def.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, def.Command, args...)

	// Merge environment
	if len(def.Env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range def.Env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}

	// Pipe stdin if configured
	if def.Stdin != "" {
		stdinVal := substituteString(def.Stdin, params)
		cmd.Stdin = strings.NewReader(stdinVal)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	result := &Result{
		Output:   stdout.String(),
		Error:    stderr.String(),
		Duration: duration,
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = 1
			result.Error = err.Error()
		}
	}

	return result, nil
}

func substituteString(tmpl string, params map[string]string) string {
	result := tmpl
	for k, v := range params {
		result = strings.ReplaceAll(result, "{"+k+"}", v)
	}
	return result
}

func substituteArgs(templates []string, params map[string]string) []string {
	args := make([]string, len(templates))
	for i, tmpl := range templates {
		result := tmpl
		for k, v := range params {
			result = strings.ReplaceAll(result, "{"+k+"}", v)
		}
		args[i] = result
	}
	return args
}
