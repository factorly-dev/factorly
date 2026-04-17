// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package vault

// FallbackBackend wraps two vault backends. Get checks the primary first,
// falls back to secondary (opened lazily on first fallback). Set/Delete/List
// operate on primary only.
type FallbackBackend struct {
	Primary       Backend
	Secondary     Backend
	SecondaryOpen func() (Backend, error) // lazy opener for secondary vault
}

func (f *FallbackBackend) ensureSecondary() Backend {
	if f.Secondary != nil {
		return f.Secondary
	}
	if f.SecondaryOpen != nil {
		b, err := f.SecondaryOpen()
		if err == nil {
			f.Secondary = b
		}
		f.SecondaryOpen = nil // only try once
	}
	return f.Secondary
}

func (f *FallbackBackend) Get(key string) (string, error) {
	if f.Primary != nil {
		val, err := f.Primary.Get(key)
		if err == nil {
			return val, nil
		}
	}
	if sec := f.ensureSecondary(); sec != nil {
		return sec.Get(key)
	}
	return "", ErrNotFound
}

func (f *FallbackBackend) Set(key, value string) error {
	if f.Primary != nil {
		return f.Primary.Set(key, value)
	}
	if sec := f.ensureSecondary(); sec != nil {
		return sec.Set(key, value)
	}
	return ErrNotFound
}

func (f *FallbackBackend) Delete(key string) error {
	if f.Primary != nil {
		return f.Primary.Delete(key)
	}
	if sec := f.ensureSecondary(); sec != nil {
		return sec.Delete(key)
	}
	return ErrNotFound
}

func (f *FallbackBackend) List() ([]string, error) {
	if f.Primary != nil {
		return f.Primary.List()
	}
	if sec := f.ensureSecondary(); sec != nil {
		return sec.List()
	}
	return nil, nil
}

// Has returns true if the key exists in either vault.
func (f *FallbackBackend) Has(key string) bool {
	if f.Primary != nil {
		if lb, ok := f.Primary.(*LocalBackend); ok && lb.Has(key) {
			return true
		}
	}
	if sec := f.ensureSecondary(); sec != nil {
		if lb, ok := sec.(*LocalBackend); ok && lb.Has(key) {
			return true
		}
	}
	return false
}

func (f *FallbackBackend) Close() error {
	if f.Primary != nil {
		f.Primary.Close()
	}
	if f.Secondary != nil {
		f.Secondary.Close()
	}
	return nil
}
