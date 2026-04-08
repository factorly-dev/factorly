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
