// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/hkdf"
)

const (
	saltLen      = 32
	nonceLen     = 12
	keyLen       = 32
	argonTime    = 2
	argonMemory  = 128 * 1024
	argonThreads = 4

	entrySaltLen = 16
	hkdfInfo     = "factorly-vault-entry-v2"
)

// vaultData is the v1 format (kept for migration).
type vaultData struct {
	Version int               `json:"version"`
	Secrets map[string]string `json:"secrets"`
}

// encryptedEntry is a single per-entry encrypted value.
type encryptedEntry struct {
	Salt       []byte `json:"salt"`       // 16 bytes, HKDF salt
	Nonce      []byte `json:"nonce"`      // 12 bytes, AES-GCM nonce
	Ciphertext []byte `json:"ciphertext"` // AES-256-GCM output
}

// vaultIndex is the v2 format: key names are visible, values are per-entry encrypted.
type vaultIndex struct {
	Version int                       `json:"version"`
	Entries map[string]encryptedEntry `json:"entries"`
}

// LocalBackend stores secrets encrypted on disk using AES-256-GCM with
// an Argon2id-derived key. Values are per-entry encrypted via HKDF.
// File locks are per-operation: shared for reads, exclusive for writes.
// No lock is held between operations, allowing concurrent readers.
type LocalBackend struct {
	path     string
	key      []byte // master key (Argon2id-derived, retained for HKDF)
	salt     []byte // file-level Argon2 salt
	mu       sync.RWMutex
	index    *vaultIndex
	dirty    bool
	lockPath string // path to .lock file
}

// DefaultVaultPath returns the default vault file location.
func DefaultVaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "vault.enc"
	}
	return filepath.Join(home, ".config", "factorly", "vault.enc")
}

// OpenLocal opens or creates an encrypted vault at the default path.
// The password slice is zeroed after key derivation.
func OpenLocal(password []byte) (*LocalBackend, error) {
	return OpenLocalAt(DefaultVaultPath(), password)
}

// OpenLocalAt opens or creates an encrypted vault at the given path.
// The password slice is zeroed after key derivation.
// Acquires a shared lock to read, releases after loading into memory.
func OpenLocalAt(path string, password []byte) (*LocalBackend, error) {
	b := &LocalBackend{
		path:     path,
		lockPath: path + ".lock",
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		zeroize(password)
		return nil, fmt.Errorf("creating vault directory: %w", err)
	}

	data, err := b.readWithSharedLock()
	if err != nil {
		if !os.IsNotExist(err) {
			zeroize(password)
			return nil, fmt.Errorf("reading vault: %w", err)
		}
		// New vault — create v2 directly
		b.salt = make([]byte, saltLen)
		if _, err := rand.Read(b.salt); err != nil {
			zeroize(password)
			return nil, fmt.Errorf("generating salt: %w", err)
		}
		b.key = deriveKey(password, b.salt)
		b.index = &vaultIndex{Version: 2, Entries: make(map[string]encryptedEntry)}
		return b, nil
	}

	// Existing vault — decrypt outer layer
	if len(data) < saltLen+nonceLen+1 {
		zeroize(password)
		return nil, fmt.Errorf("vault file is corrupt (too small)")
	}

	b.salt = data[:saltLen]
	b.key = deriveKey(password, b.salt)

	nonce := data[saltLen : saltLen+nonceLen]
	ciphertext := data[saltLen+nonceLen:]

	block, err := aes.NewCipher(b.key)
	if err != nil {
		return nil, fmt.Errorf("creating cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypting vault (wrong password?): %w", err)
	}

	// Detect version
	var probe struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(plaintext, &probe); err != nil {
		return nil, fmt.Errorf("parsing vault version: %w", err)
	}

	switch probe.Version {
	case 0, 1:
		// Migrate v1 → v2: decrypt all values, re-encrypt per-entry
		var vd vaultData
		if err := json.Unmarshal(plaintext, &vd); err != nil {
			return nil, fmt.Errorf("parsing v1 vault: %w", err)
		}
		b.index = &vaultIndex{Version: 2, Entries: make(map[string]encryptedEntry, len(vd.Secrets))}
		for k, v := range vd.Secrets {
			entry, err := encryptValue(b.key, v)
			if err != nil {
				return nil, fmt.Errorf("migrating entry %q: %w", k, err)
			}
			b.index.Entries[k] = entry
		}
		b.dirty = true
		if err := b.save(); err != nil {
			return nil, fmt.Errorf("saving migrated vault: %w", err)
		}

	case 2:
		var vi vaultIndex
		if err := json.Unmarshal(plaintext, &vi); err != nil {
			return nil, fmt.Errorf("parsing v2 vault: %w", err)
		}
		if vi.Entries == nil {
			vi.Entries = make(map[string]encryptedEntry)
		}
		b.index = &vi

	default:
		return nil, fmt.Errorf("unsupported vault version %d", probe.Version)
	}

	return b, nil
}

func (b *LocalBackend) Get(key string) (string, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	entry, ok := b.index.Entries[key]
	if !ok {
		return "", ErrNotFound
	}
	return decryptValue(b.key, entry)
}

// Has returns true if a key exists in the vault.
func (b *LocalBackend) Has(key string) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	_, ok := b.index.Entries[key]
	return ok
}

func (b *LocalBackend) Set(key, value string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	entry, err := encryptValue(b.key, value)
	if err != nil {
		return fmt.Errorf("encrypting entry: %w", err)
	}
	b.index.Entries[key] = entry
	b.dirty = true
	return b.save()
}

func (b *LocalBackend) Delete(key string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, ok := b.index.Entries[key]; !ok {
		return ErrNotFound
	}
	delete(b.index.Entries, key)
	b.dirty = true
	return b.save()
}

func (b *LocalBackend) List() ([]string, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	keys := make([]string, 0, len(b.index.Entries))
	for k := range b.index.Entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys, nil
}

func (b *LocalBackend) Close() error {
	zeroize(b.key)
	zeroize(b.salt)
	return nil
}

// readWithSharedLock reads the vault file while holding a shared lock.
// Returns os.ErrNotExist if the file doesn't exist.
func (b *LocalBackend) readWithSharedLock() ([]byte, error) {
	lockFile, err := os.OpenFile(b.lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("opening lock file: %w", err)
	}
	defer lockFile.Close()

	if err := lockFileShared(lockFile); err != nil {
		return nil, fmt.Errorf("acquiring shared lock: %w", err)
	}
	defer func() { _ = unlockFile(lockFile) }()

	return os.ReadFile(b.path)
}

// withExclusiveLock runs fn while holding an exclusive file lock.
func (b *LocalBackend) withExclusiveLock(fn func() error) error {
	lockFile, err := os.OpenFile(b.lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("opening lock file: %w", err)
	}
	defer lockFile.Close()

	if err := lockFileExclusive(lockFile); err != nil {
		return fmt.Errorf("acquiring exclusive lock: %w", err)
	}
	defer func() { _ = unlockFile(lockFile) }()

	return fn()
}

func (b *LocalBackend) save() error {
	plaintext, err := json.Marshal(b.index)
	if err != nil {
		return fmt.Errorf("marshaling vault: %w", err)
	}

	block, err := aes.NewCipher(b.key)
	if err != nil {
		return fmt.Errorf("creating cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("creating GCM: %w", err)
	}

	nonce := make([]byte, nonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("generating nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	// Build file: salt + nonce + ciphertext
	out := make([]byte, 0, saltLen+nonceLen+len(ciphertext))
	out = append(out, b.salt...)
	out = append(out, nonce...)
	out = append(out, ciphertext...)

	if err := os.MkdirAll(filepath.Dir(b.path), 0o755); err != nil {
		return fmt.Errorf("creating vault directory: %w", err)
	}

	// Acquire exclusive lock for atomic write
	return b.withExclusiveLock(func() error {
		tmp := b.path + ".tmp"
		if err := os.WriteFile(tmp, out, 0o600); err != nil {
			return fmt.Errorf("writing vault temp file: %w", err)
		}
		if err := os.Rename(tmp, b.path); err != nil {
			os.Remove(tmp)
			return fmt.Errorf("renaming vault file: %w", err)
		}
		return nil
	})
}

// --- Key derivation ---

func deriveKey(password []byte, salt []byte) []byte {
	key := argon2.IDKey(password, salt, argonTime, argonMemory, argonThreads, keyLen)
	zeroize(password) // clear password from memory after derivation
	return key
}

func deriveEntryKey(masterKey, entrySalt []byte) ([]byte, error) {
	r := hkdf.New(sha256.New, masterKey, entrySalt, []byte(hkdfInfo))
	key := make([]byte, keyLen)
	if _, err := io.ReadFull(r, key); err != nil {
		return nil, fmt.Errorf("deriving entry key: %w", err)
	}
	return key, nil
}

// --- Per-entry encrypt/decrypt ---

func encryptValue(masterKey []byte, plaintext string) (encryptedEntry, error) {
	salt := make([]byte, entrySaltLen)
	if _, err := rand.Read(salt); err != nil {
		return encryptedEntry{}, fmt.Errorf("generating entry salt: %w", err)
	}

	entryKey, err := deriveEntryKey(masterKey, salt)
	if err != nil {
		return encryptedEntry{}, err
	}
	defer zeroize(entryKey)

	block, err := aes.NewCipher(entryKey)
	if err != nil {
		return encryptedEntry{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return encryptedEntry{}, err
	}

	nonce := make([]byte, nonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return encryptedEntry{}, fmt.Errorf("generating entry nonce: %w", err)
	}

	ct := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	return encryptedEntry{Salt: salt, Nonce: nonce, Ciphertext: ct}, nil
}

func decryptValue(masterKey []byte, entry encryptedEntry) (string, error) {
	entryKey, err := deriveEntryKey(masterKey, entry.Salt)
	if err != nil {
		return "", err
	}
	defer zeroize(entryKey)

	block, err := aes.NewCipher(entryKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	plaintext, err := gcm.Open(nil, entry.Nonce, entry.Ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypting entry: %w", err)
	}
	return string(plaintext), nil
}

func zeroize(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
