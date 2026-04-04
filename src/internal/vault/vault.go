package vault

import "errors"

// ErrNotFound is returned when a secret key does not exist in the backend.
var ErrNotFound = errors.New("secret not found")

// Backend is the interface all secret providers implement.
type Backend interface {
	Get(key string) (string, error)
	Set(key, value string) error
	Delete(key string) error
	List() ([]string, error)
	Close() error
}
