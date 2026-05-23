// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package vault

import "errors"

// ErrNotFound is returned when a secret key does not exist in the backend.
var ErrNotFound = errors.New("secret not found")

// ErrWrongPassword wraps every authentication-failure path returned by
// OpenLocalAt so callers can branch on `errors.Is(err, ErrWrongPassword)`
// to decide whether to re-prompt. Other open errors (corrupted file,
// I/O failure, missing path) intentionally do NOT wrap this sentinel —
// retrying with a new password won't help those.
var ErrWrongPassword = errors.New("wrong vault password")

// Backend is the interface all secret providers implement.
type Backend interface {
	Get(key string) (string, error)
	Set(key, value string) error
	Delete(key string) error
	List() ([]string, error)
	Close() error
}
