package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"golang.org/x/crypto/argon2"
)

const (
	saltLen      = 32
	nonceLen     = 12
	keyLen       = 32
	argonTime    = 2
	argonMemory  = 128 * 1024
	argonThreads = 4
)

type vaultData struct {
	Version int               `json:"version"`
	Secrets map[string]string `json:"secrets"`
}

// LocalBackend stores secrets encrypted on disk using AES-256-GCM with
// an Argon2id-derived key.
type LocalBackend struct {
	path  string
	key   []byte
	salt  []byte
	mu    sync.RWMutex
	data  *vaultData
	dirty bool
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
func OpenLocal(password string) (*LocalBackend, error) {
	return OpenLocalAt(DefaultVaultPath(), password)
}

// OpenLocalAt opens or creates an encrypted vault at the given path.
func OpenLocalAt(path, password string) (*LocalBackend, error) {
	b := &LocalBackend{path: path}

	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("reading vault: %w", err)
		}
		// New vault — generate fresh salt
		b.salt = make([]byte, saltLen)
		if _, err := rand.Read(b.salt); err != nil {
			return nil, fmt.Errorf("generating salt: %w", err)
		}
		b.key = deriveKey(password, b.salt)
		b.data = &vaultData{Version: 1, Secrets: make(map[string]string)}
		return b, nil
	}

	// Existing vault — decrypt
	if len(data) < saltLen+nonceLen+1 {
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

	var vd vaultData
	if err := json.Unmarshal(plaintext, &vd); err != nil {
		return nil, fmt.Errorf("parsing vault data: %w", err)
	}
	if vd.Secrets == nil {
		vd.Secrets = make(map[string]string)
	}

	b.data = &vd
	return b, nil
}

func (b *LocalBackend) Get(key string) (string, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	val, ok := b.data.Secrets[key]
	if !ok {
		return "", ErrNotFound
	}
	return val, nil
}

func (b *LocalBackend) Set(key, value string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.data.Secrets[key] = value
	b.dirty = true
	return b.save()
}

func (b *LocalBackend) Delete(key string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, ok := b.data.Secrets[key]; !ok {
		return ErrNotFound
	}
	delete(b.data.Secrets, key)
	b.dirty = true
	return b.save()
}

func (b *LocalBackend) List() ([]string, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	keys := make([]string, 0, len(b.data.Secrets))
	for k := range b.data.Secrets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys, nil
}

func (b *LocalBackend) Close() error {
	return nil
}

func (b *LocalBackend) save() error {
	plaintext, err := json.Marshal(b.data)
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

	return os.WriteFile(b.path, out, 0o600)
}

func deriveKey(password string, salt []byte) []byte {
	return argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, keyLen)
}
