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
	Command     string
	Args        []string
	Stdin       string // template with {{param}} placeholders, piped to subprocess stdin
	Interactive bool   // connect subprocess to terminal (stdin/stdout/stderr)
	Env         map[string]string
	EnvStrict   bool // when true, child gets minimal env instead of inheriting parent
	Timeout     time.Duration
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

	// Check for unresolved placeholders in templates before substitution.
	for _, tmpl := range def.Args {
		remaining := tmpl
		for k := range params {
			remaining = strings.ReplaceAll(remaining, "{{"+k+"}}", "")
		}
		if idx := strings.Index(remaining, "{{"); idx != -1 {
			end := strings.Index(remaining[idx:], "}}")
			if end != -1 {
				placeholder := remaining[idx+2 : idx+end]
				return nil, fmt.Errorf("cli provider: unresolved parameter {{%s}} in tool %q", placeholder, toolName)
			}
		}
	}

	args := substituteArgs(def.Args, params)

	timeout := def.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, def.Command, args...)

	// Build environment: strict = minimal base, standard = full parent
	cmd.Env = buildEnv(def.Env, def.EnvStrict)

	// Interactive mode: connect directly to terminal
	if def.Interactive {
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		start := time.Now()
		err := cmd.Run()
		duration := time.Since(start)

		result := &Result{Duration: duration}
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
		result = strings.ReplaceAll(result, "{{"+k+"}}", v)
	}
	return result
}

func substituteArgs(templates []string, params map[string]string) []string {
	args := make([]string, len(templates))
	for i, tmpl := range templates {
		result := tmpl
		for k, v := range params {
			result = strings.ReplaceAll(result, "{{"+k+"}}", v)
		}
		args[i] = result
	}
	return args
}
