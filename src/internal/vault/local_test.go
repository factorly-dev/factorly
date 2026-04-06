package vault

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalNewVault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.enc")
	b, err := OpenLocalAt(path, "password123")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	keys, _ := b.List()
	if len(keys) != 0 {
		t.Errorf("expected empty vault, got %d keys", len(keys))
	}
}

func TestLocalSetAndGet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.enc")
	b, err := OpenLocalAt(path, "password123")
	if err != nil {
		t.Fatal(err)
	}

	if err := b.Set("TOKEN", "secret-value"); err != nil {
		t.Fatal(err)
	}

	val, err := b.Get("TOKEN")
	if err != nil {
		t.Fatal(err)
	}
	if val != "secret-value" {
		t.Errorf("expected 'secret-value', got %q", val)
	}
	b.Close()

	// Re-open with same password — should decrypt and find the secret
	b2, err := OpenLocalAt(path, "password123")
	if err != nil {
		t.Fatal(err)
	}
	defer b2.Close()

	val2, err := b2.Get("TOKEN")
	if err != nil {
		t.Fatal(err)
	}
	if val2 != "secret-value" {
		t.Errorf("expected 'secret-value' after re-open, got %q", val2)
	}
}

func TestLocalGetNotFound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.enc")
	b, err := OpenLocalAt(path, "password123")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	_, err = b.Get("MISSING")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestLocalDelete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.enc")
	b, err := OpenLocalAt(path, "password123")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	_ = b.Set("KEY", "value")

	if err := b.Delete("KEY"); err != nil {
		t.Fatal(err)
	}

	_, err = b.Get("KEY")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestLocalDeleteNotFound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.enc")
	b, err := OpenLocalAt(path, "password123")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	err = b.Delete("MISSING")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestLocalList(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.enc")
	b, err := OpenLocalAt(path, "password123")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	_ = b.Set("ZEBRA", "z")
	_ = b.Set("ALPHA", "a")
	_ = b.Set("MIDDLE", "m")

	keys, err := b.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(keys))
	}
	if keys[0] != "ALPHA" || keys[1] != "MIDDLE" || keys[2] != "ZEBRA" {
		t.Errorf("expected sorted keys, got %v", keys)
	}
}

func TestLocalMultipleSecrets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.enc")
	b, err := OpenLocalAt(path, "pw")
	if err != nil {
		t.Fatal(err)
	}

	_ = b.Set("A", "val-a")
	_ = b.Set("B", "val-b")
	_ = b.Set("C", "val-c")
	b.Close()

	b2, err := OpenLocalAt(path, "pw")
	if err != nil {
		t.Fatal(err)
	}
	defer b2.Close()

	for _, tc := range []struct{ key, want string }{{"A", "val-a"}, {"B", "val-b"}, {"C", "val-c"}} {
		v, err := b2.Get(tc.key)
		if err != nil {
			t.Errorf("Get(%s): %v", tc.key, err)
		}
		if v != tc.want {
			t.Errorf("Get(%s) = %q, want %q", tc.key, v, tc.want)
		}
	}
}

func TestLocalWrongPassword(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.enc")
	b, err := OpenLocalAt(path, "correct-password")
	if err != nil {
		t.Fatal(err)
	}
	_ = b.Set("SECRET", "value")
	b.Close()

	_, err = OpenLocalAt(path, "wrong-password")
	if err == nil {
		t.Fatal("expected error for wrong password")
	}
}

func TestLocalOverwriteSecret(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.enc")
	b, err := OpenLocalAt(path, "pw")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	_ = b.Set("KEY", "first")
	_ = b.Set("KEY", "second")

	val, _ := b.Get("KEY")
	if val != "second" {
		t.Errorf("expected overwritten value 'second', got %q", val)
	}
}

func TestLocalFilePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.enc")
	b, err := OpenLocalAt(path, "pw")
	if err != nil {
		t.Fatal(err)
	}
	_ = b.Set("KEY", "value")
	b.Close()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	perm := info.Mode().Perm()
	if perm != 0o600 {
		t.Errorf("expected vault file permissions 0600, got %04o", perm)
	}
}

func TestLocalCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.enc")
	_ = os.WriteFile(path, []byte("not encrypted data that is long enough to pass size check plus more"), 0o600)

	_, err := OpenLocalAt(path, "password")
	if err == nil {
		t.Fatal("expected error for corrupt vault file")
	}
}

func TestLocalTooSmallFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.enc")
	_ = os.WriteFile(path, []byte("tiny"), 0o600)

	_, err := OpenLocalAt(path, "password")
	if err == nil {
		t.Fatal("expected error for too-small vault file")
	}
}

func TestLocalCreatesDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "deep", "vault.enc")
	b, err := OpenLocalAt(path, "pw")
	if err != nil {
		t.Fatal(err)
	}
	_ = b.Set("KEY", "value")
	b.Close()

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected vault file to be created in nested directory")
	}
}

// --- V2 Per-Entry Encryption Tests ---

// writeV1Vault creates a v1-format vault file for migration testing.
func writeV1Vault(t *testing.T, path, password string, secrets map[string]string) {
	t.Helper()
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		t.Fatal(err)
	}
	key := deriveKey(password, salt)

	vd := vaultData{Version: 1, Secrets: secrets}
	plaintext, err := json.Marshal(vd)
	if err != nil {
		t.Fatal(err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, nonceLen)
	if _, err := rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	ct := gcm.Seal(nil, nonce, plaintext, nil)

	out := make([]byte, 0, saltLen+nonceLen+len(ct))
	out = append(out, salt...)
	out = append(out, nonce...)
	out = append(out, ct...)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, out, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestV1Migration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.enc")
	writeV1Vault(t, path, "pw", map[string]string{"TOKEN": "secret123"})

	b, err := OpenLocalAt(path, "pw")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	val, err := b.Get("TOKEN")
	if err != nil {
		t.Fatal(err)
	}
	if val != "secret123" {
		t.Errorf("expected 'secret123', got %q", val)
	}
}

func TestV1MigrationPreservesValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.enc")
	secrets := map[string]string{
		"TOKEN":   "secret123",
		"API_KEY": "key456",
		"DB_PASS": "dbpass789",
	}
	writeV1Vault(t, path, "pw", secrets)

	b, err := OpenLocalAt(path, "pw")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	for k, want := range secrets {
		got, err := b.Get(k)
		if err != nil {
			t.Errorf("Get(%s): %v", k, err)
			continue
		}
		if got != want {
			t.Errorf("Get(%s) = %q, want %q", k, got, want)
		}
	}
}

func TestV1MigrationIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.enc")
	writeV1Vault(t, path, "pw", map[string]string{"TOKEN": "secret"})

	// First open triggers migration
	b, err := OpenLocalAt(path, "pw")
	if err != nil {
		t.Fatal(err)
	}
	b.Close()

	// Second open should read v2 directly
	b2, err := OpenLocalAt(path, "pw")
	if err != nil {
		t.Fatal(err)
	}
	defer b2.Close()

	val, err := b2.Get("TOKEN")
	if err != nil {
		t.Fatal(err)
	}
	if val != "secret" {
		t.Errorf("expected 'secret', got %q", val)
	}

	// Verify it's v2 by checking the index version
	if b2.index.Version != 2 {
		t.Errorf("expected version 2, got %d", b2.index.Version)
	}
}

func TestPerEntryUniqueCiphertext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.enc")
	b, err := OpenLocalAt(path, "pw")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	// Set two keys to the same value
	_ = b.Set("KEY_A", "identical-value")
	_ = b.Set("KEY_B", "identical-value")

	entryA := b.index.Entries["KEY_A"]
	entryB := b.index.Entries["KEY_B"]

	// Salts must differ
	if bytes.Equal(entryA.Salt, entryB.Salt) {
		t.Error("expected different salts for different entries")
	}

	// Ciphertext must differ (different salt → different key → different ciphertext)
	if bytes.Equal(entryA.Ciphertext, entryB.Ciphertext) {
		t.Error("expected different ciphertext for entries with same value")
	}
}

func TestOverwriteRegeneratesSalt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.enc")
	b, err := OpenLocalAt(path, "pw")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	_ = b.Set("KEY", "first")
	saltBefore := make([]byte, len(b.index.Entries["KEY"].Salt))
	copy(saltBefore, b.index.Entries["KEY"].Salt)

	_ = b.Set("KEY", "second")
	saltAfter := b.index.Entries["KEY"].Salt

	if bytes.Equal(saltBefore, saltAfter) {
		t.Error("expected different salt after overwrite")
	}
}

func TestDecryptValueIsolation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.enc")
	b, err := OpenLocalAt(path, "pw")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	_ = b.Set("GOOD_A", "value-a")
	_ = b.Set("CORRUPT", "will-be-corrupted")
	_ = b.Set("GOOD_B", "value-b")

	// Corrupt one entry's ciphertext
	entry := b.index.Entries["CORRUPT"]
	entry.Ciphertext = []byte("garbage")
	b.index.Entries["CORRUPT"] = entry

	// Other entries should still decrypt fine
	valA, err := b.Get("GOOD_A")
	if err != nil {
		t.Errorf("Get(GOOD_A) should succeed: %v", err)
	}
	if valA != "value-a" {
		t.Errorf("expected 'value-a', got %q", valA)
	}

	valB, err := b.Get("GOOD_B")
	if err != nil {
		t.Errorf("Get(GOOD_B) should succeed: %v", err)
	}
	if valB != "value-b" {
		t.Errorf("expected 'value-b', got %q", valB)
	}

	// Corrupted entry should fail
	_, err = b.Get("CORRUPT")
	if err == nil {
		t.Error("expected error for corrupted entry")
	}
}

func TestDeriveEntryKey(t *testing.T) {
	masterKey := make([]byte, keyLen)
	if _, err := rand.Read(masterKey); err != nil {
		t.Fatal(err)
	}

	salt1 := make([]byte, entrySaltLen)
	if _, err := rand.Read(salt1); err != nil {
		t.Fatal(err)
	}
	salt2 := make([]byte, entrySaltLen)
	if _, err := rand.Read(salt2); err != nil {
		t.Fatal(err)
	}

	// Same inputs → same key
	key1a, err := deriveEntryKey(masterKey, salt1)
	if err != nil {
		t.Fatal(err)
	}
	key1b, err := deriveEntryKey(masterKey, salt1)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(key1a, key1b) {
		t.Error("same master key + same salt should produce same entry key")
	}

	// Different salt → different key
	key2, err := deriveEntryKey(masterKey, salt2)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(key1a, key2) {
		t.Error("different salt should produce different entry key")
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	masterKey := make([]byte, keyLen)
	if _, err := rand.Read(masterKey); err != nil {
		t.Fatal(err)
	}

	values := []string{"hello", "", "special chars: !@#$%^&*()", "unicode: 日本語"}
	for _, val := range values {
		entry, err := encryptValue(masterKey, val)
		if err != nil {
			t.Fatalf("encryptValue(%q): %v", val, err)
		}
		got, err := decryptValue(masterKey, entry)
		if err != nil {
			t.Fatalf("decryptValue(%q): %v", val, err)
		}
		if got != val {
			t.Errorf("round trip: expected %q, got %q", val, got)
		}
	}
}

func TestNewVaultIsV2(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.enc")
	b, err := OpenLocalAt(path, "pw")
	if err != nil {
		t.Fatal(err)
	}
	_ = b.Set("KEY", "value")
	b.Close()

	// Re-open and check version
	b2, err := OpenLocalAt(path, "pw")
	if err != nil {
		t.Fatal(err)
	}
	defer b2.Close()

	if b2.index.Version != 2 {
		t.Errorf("expected version 2, got %d", b2.index.Version)
	}
}

func TestUnsupportedVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.enc")

	// Create a vault with version 99
	salt := make([]byte, saltLen)
	_, _ = rand.Read(salt)
	key := deriveKey("pw", salt)

	vd := struct {
		Version int `json:"version"`
	}{Version: 99}
	plaintext, _ := json.Marshal(vd)

	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)
	nonce := make([]byte, nonceLen)
	_, _ = rand.Read(nonce)
	ct := gcm.Seal(nil, nonce, plaintext, nil)

	out := make([]byte, 0, saltLen+nonceLen+len(ct))
	out = append(out, salt...)
	out = append(out, nonce...)
	out = append(out, ct...)
	_ = os.WriteFile(path, out, 0o600)

	_, err := OpenLocalAt(path, "pw")
	if err == nil {
		t.Fatal("expected error for unsupported version")
	}
}

func TestConcurrentReadsAllowed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.enc")
	b1, err := OpenLocalAt(path, "pw")
	if err != nil {
		t.Fatal(err)
	}
	_ = b1.Set("KEY", "value")
	b1.Close()

	// Two concurrent opens should both succeed (shared read lock)
	b2, err := OpenLocalAt(path, "pw")
	if err != nil {
		t.Fatal(err)
	}
	b3, err := OpenLocalAt(path, "pw")
	if err != nil {
		t.Fatal(err)
	}

	// Both should be able to read
	val2, err := b2.Get("KEY")
	if err != nil {
		t.Fatal(err)
	}
	val3, err := b3.Get("KEY")
	if err != nil {
		t.Fatal(err)
	}
	if val2 != "value" || val3 != "value" {
		t.Errorf("expected 'value' from both, got %q and %q", val2, val3)
	}

	b2.Close()
	b3.Close()
}

func TestConcurrentWritesSerialized(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.enc")
	b1, err := OpenLocalAt(path, "pw")
	if err != nil {
		t.Fatal(err)
	}
	defer b1.Close()

	// Write from two goroutines — both should succeed (serialized by exclusive lock)
	done := make(chan error, 2)
	go func() {
		done <- b1.Set("KEY_A", "value-a")
	}()
	go func() {
		done <- b1.Set("KEY_B", "value-b")
	}()

	for range 2 {
		if err := <-done; err != nil {
			t.Fatalf("concurrent write failed: %v", err)
		}
	}

	// Both values should be present
	valA, _ := b1.Get("KEY_A")
	valB, _ := b1.Get("KEY_B")
	if valA != "value-a" {
		t.Errorf("expected value-a, got %q", valA)
	}
	if valB != "value-b" {
		t.Errorf("expected value-b, got %q", valB)
	}
}

func TestAtomicWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.enc")
	b, err := OpenLocalAt(path, "pw")
	if err != nil {
		t.Fatal(err)
	}

	_ = b.Set("KEY", "value")
	b.Close()

	// Verify no temp file left behind
	tmpPath := path + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("expected temp file to be cleaned up after atomic write")
	}

	// Verify vault is readable
	b2, err := OpenLocalAt(path, "pw")
	if err != nil {
		t.Fatal(err)
	}
	defer b2.Close()

	val, err := b2.Get("KEY")
	if err != nil {
		t.Fatal(err)
	}
	if val != "value" {
		t.Errorf("expected 'value', got %q", val)
	}
}

func TestCloseZeroizesKeyAndSalt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.enc")
	b, err := OpenLocalAt(path, "pw")
	if err != nil {
		t.Fatal(err)
	}
	_ = b.Set("KEY", "value")

	// Grab references before close
	key := b.key
	salt := b.salt

	b.Close()

	// Key should be zeroed
	allZero := true
	for _, v := range key {
		if v != 0 {
			allZero = false
			break
		}
	}
	if !allZero {
		t.Error("expected master key to be zeroized after Close")
	}

	// Salt should be zeroed
	allZero = true
	for _, v := range salt {
		if v != 0 {
			allZero = false
			break
		}
	}
	if !allZero {
		t.Error("expected salt to be zeroized after Close")
	}
}
