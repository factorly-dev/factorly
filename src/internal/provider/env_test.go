package provider

import (
	"os"
	"strings"
	"testing"
)

func TestBaseEnvIncludesEssentials(t *testing.T) {
	env := baseEnv()
	envMap := envToMap(env)

	// PATH should always be present (needed to find binaries)
	if _, ok := envMap["PATH"]; !ok {
		t.Error("expected PATH in base env")
	}

	// Should NOT contain common secret-bearing vars
	for _, bad := range []string{"AWS_SECRET_ACCESS_KEY", "GITHUB_TOKEN", "NPM_TOKEN", "SSH_AUTH_SOCK"} {
		if _, ok := envMap[bad]; ok {
			t.Errorf("base env should not contain %s", bad)
		}
	}
}

func TestBaseEnvLimited(t *testing.T) {
	env := baseEnv()
	// Should be a small set — at most PATH, HOME, USER, LANG, TERM
	if len(env) > 5 {
		t.Errorf("expected at most 5 base env vars, got %d: %v", len(env), env)
	}
}

func TestBuildEnvExplicit(t *testing.T) {
	env := buildEnv(map[string]string{"FOO": "bar", "BAZ": "qux"}, nil)
	envMap := envToMap(env)

	if envMap["FOO"] != "bar" {
		t.Errorf("expected FOO=bar, got %q", envMap["FOO"])
	}
	if envMap["BAZ"] != "qux" {
		t.Errorf("expected BAZ=qux, got %q", envMap["BAZ"])
	}
}

func TestBuildEnvPassthrough(t *testing.T) {
	os.Setenv("TEST_PASSTHROUGH_VAR", "secret123")
	defer os.Unsetenv("TEST_PASSTHROUGH_VAR")

	env := buildEnv(nil, []string{"TEST_PASSTHROUGH_VAR", "NONEXISTENT_VAR"})
	envMap := envToMap(env)

	if envMap["TEST_PASSTHROUGH_VAR"] != "secret123" {
		t.Errorf("expected passthrough var, got %q", envMap["TEST_PASSTHROUGH_VAR"])
	}
	if _, ok := envMap["NONEXISTENT_VAR"]; ok {
		t.Error("should not include nonexistent passthrough var")
	}
}

func TestBuildEnvExplicitOverridesBase(t *testing.T) {
	// Explicit PATH should appear (may duplicate with base, but last wins in most systems)
	env := buildEnv(map[string]string{"PATH": "/custom/bin"}, nil)

	found := false
	for _, e := range env {
		if e == "PATH=/custom/bin" {
			found = true
		}
	}
	if !found {
		t.Error("expected explicit PATH=/custom/bin in env")
	}
}

func TestBuildEnvEmpty(t *testing.T) {
	env := buildEnv(nil, nil)
	// Should still have base env
	if len(env) == 0 {
		t.Error("expected base env even with no explicit or passthrough vars")
	}
}

func TestConnectStdioRestrictedRestoresEnv(t *testing.T) {
	// Set a secret that should NOT leak to child processes
	os.Setenv("TEST_SECRET_TOKEN", "super-secret")
	defer os.Unsetenv("TEST_SECRET_TOKEN")

	// Capture env before
	envBefore := os.Environ()

	// Call connectStdioRestricted with a command that will fail immediately
	// (we don't care about the connection, just that env is restored)
	def := MCPServerDef{
		Command: "false", // exits immediately with error
		Env:     map[string]string{"CUSTOM_VAR": "hello"},
	}
	_, _ = connectStdioRestricted(def) // error expected, we don't care

	// Verify env is fully restored
	envAfter := os.Environ()
	if len(envAfter) != len(envBefore) {
		t.Errorf("env count changed: before=%d after=%d", len(envBefore), len(envAfter))
	}

	// Our secret should be back
	if v := os.Getenv("TEST_SECRET_TOKEN"); v != "super-secret" {
		t.Errorf("expected TEST_SECRET_TOKEN restored, got %q", v)
	}
}

func TestConnectStdioRestrictedExcludesSecrets(t *testing.T) {
	// This test verifies the restricted env is built correctly.
	// We can't easily inspect what the child process sees (it fails to connect),
	// but we can verify the buildEnv output excludes secrets.
	os.Setenv("TEST_LEAKED_SECRET", "should-not-appear")
	defer os.Unsetenv("TEST_LEAKED_SECRET")

	def := MCPServerDef{
		Command:        "echo",
		Env:            map[string]string{"ALLOWED": "yes"},
		EnvPassthrough: []string{"TEST_LEAKED_SECRET"}, // only if explicitly listed
	}

	env := buildEnv(def.Env, def.EnvPassthrough)
	envMap := envToMap(env)

	// Explicit var present
	if envMap["ALLOWED"] != "yes" {
		t.Error("expected ALLOWED=yes in restricted env")
	}

	// Passthrough var present (because explicitly listed)
	if envMap["TEST_LEAKED_SECRET"] != "should-not-appear" {
		t.Error("expected passthrough var in restricted env")
	}

	// Build without passthrough — secret should NOT be present
	envNoPass := buildEnv(def.Env, nil)
	envMapNoPass := envToMap(envNoPass)
	if _, ok := envMapNoPass["TEST_LEAKED_SECRET"]; ok {
		t.Error("secret should not appear without explicit passthrough")
	}
}

func TestConnectStdioRestrictedConcurrentSafe(t *testing.T) {
	// Verify the mutex prevents concurrent env corruption
	os.Setenv("TEST_CONCURRENT_VAR", "original")
	defer os.Unsetenv("TEST_CONCURRENT_VAR")

	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			def := MCPServerDef{Command: "false"}
			_, _ = connectStdioRestricted(def)
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// Env should be fully restored after all goroutines complete
	if v := os.Getenv("TEST_CONCURRENT_VAR"); v != "original" {
		t.Errorf("expected TEST_CONCURRENT_VAR=original after concurrent access, got %q", v)
	}
}

func envToMap(env []string) map[string]string {
	m := make(map[string]string)
	for _, e := range env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			m[parts[0]] = parts[1]
		}
	}
	return m
}
