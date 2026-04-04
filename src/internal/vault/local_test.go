package vault

import (
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
