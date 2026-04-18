// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package vault

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const externalTimeout = 30 * time.Second

// ExternalBackendConfig defines a tool-style vault backend.
type ExternalBackendConfig struct {
	Type string           `yaml:"type"` // "cli"
	Get  BackendOpConfig  `yaml:"get"`
	List *BackendOpConfig `yaml:"list,omitempty"`
}

// BackendOpConfig defines a single operation (get or list) for an external backend.
type BackendOpConfig struct {
	Command string   `yaml:"command"`
	Args    []string `yaml:"args,omitempty"`
}

// ExternalBackend implements the Backend interface by shelling out to CLI
// commands. Read-only — Set and Delete return errors.
type ExternalBackend struct {
	name   string
	config ExternalBackendConfig
}

// NewExternalBackend creates an external vault backend.
func NewExternalBackend(name string, config ExternalBackendConfig) *ExternalBackend {
	return &ExternalBackend{name: name, config: config}
}

// Get retrieves a secret by running the configured get command.
// {{key}} in args is replaced with the requested key.
func (b *ExternalBackend) Get(key string) (string, error) {
	if b.config.Get.Command == "" {
		return "", fmt.Errorf("backend %q has no get command configured", b.name)
	}

	args := substituteKey(b.config.Get.Args, key)

	ctx, cancel := context.WithTimeout(context.Background(), externalTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, b.config.Get.Command, args...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("backend %q get failed (exit %d): %s",
				b.name, exitErr.ExitCode(), strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", fmt.Errorf("backend %q get failed: %w", b.name, err)
	}

	return strings.TrimSpace(string(out)), nil
}

// List returns available secret names by running the configured list command.
func (b *ExternalBackend) List() ([]string, error) {
	if b.config.List == nil || b.config.List.Command == "" {
		return nil, fmt.Errorf("backend %q has no list command configured", b.name)
	}

	ctx, cancel := context.WithTimeout(context.Background(), externalTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, b.config.List.Command, b.config.List.Args...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("backend %q list failed (exit %d): %s",
				b.name, exitErr.ExitCode(), strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("backend %q list failed: %w", b.name, err)
	}

	return splitLines(string(out)), nil
}

// Set is not supported — external backends are read-only.
func (b *ExternalBackend) Set(_, _ string) error {
	return fmt.Errorf("backend %q is read-only — manage secrets in %s directly", b.name, b.name)
}

// Delete is not supported — external backends are read-only.
func (b *ExternalBackend) Delete(_ string) error {
	return fmt.Errorf("backend %q is read-only — manage secrets in %s directly", b.name, b.name)
}

// Close is a no-op for external backends.
func (b *ExternalBackend) Close() error {
	return nil
}

// substituteKey replaces {{key}} in a list of args with the actual key value.
func substituteKey(args []string, key string) []string {
	result := make([]string, len(args))
	for i, arg := range args {
		result[i] = strings.ReplaceAll(arg, "{{key}}", key)
	}
	return result
}

// splitLines splits output by newlines, trimming empty lines.
func splitLines(s string) []string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	var result []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}
