// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package vault

import (
	"fmt"
	"os"
)

// EnvBackend resolves secrets from environment variables.
// Registered as the "env" backend in the resolver.
type EnvBackend struct{}

func (EnvBackend) Get(key string) (string, error) {
	if val, ok := os.LookupEnv(key); ok {
		return val, nil
	}
	return "", ErrNotFound
}

func (EnvBackend) Set(_, _ string) error   { return fmt.Errorf("env backend is read-only") }
func (EnvBackend) Delete(_ string) error   { return fmt.Errorf("env backend is read-only") }
func (EnvBackend) List() ([]string, error) { return nil, fmt.Errorf("env backend is read-only") }
func (EnvBackend) Close() error            { return nil }

// EnvBackendWithOverrides wraps EnvBackend with additional key-value overrides.
// Overrides are checked first, then falls back to os.LookupEnv.
type EnvBackendWithOverrides struct {
	Overrides map[string]string
}

func (b EnvBackendWithOverrides) Get(key string) (string, error) {
	if val, ok := b.Overrides[key]; ok {
		return val, nil
	}
	return EnvBackend{}.Get(key)
}

func (b EnvBackendWithOverrides) Set(_, _ string) error {
	return fmt.Errorf("env backend is read-only")
}
func (b EnvBackendWithOverrides) Delete(_ string) error {
	return fmt.Errorf("env backend is read-only")
}
func (b EnvBackendWithOverrides) List() ([]string, error) {
	return nil, fmt.Errorf("env backend is read-only")
}
func (b EnvBackendWithOverrides) Close() error { return nil }
