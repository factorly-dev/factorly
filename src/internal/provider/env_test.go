package provider

import (
	"os"
	"strings"
	"testing"
)

func TestBaseEnvIncludesEssentials(t *testing.T) {
	env := baseEnv()
	envMap := envToMap(env)

	if _, ok := envMap["PATH"]; !ok {
		t.Error("expected PATH in base env")
	}

	for _, bad := range []string{"AWS_SECRET_ACCESS_KEY", "GITHUB_TOKEN", "NPM_TOKEN", "SSH_AUTH_SOCK"} {
		if _, ok := envMap[bad]; ok {
			t.Errorf("base env should not contain %s", bad)
		}
	}
}

func TestBaseEnvLimited(t *testing.T) {
	env := baseEnv()
	if len(env) > 5 {
		t.Errorf("expected at most 5 base env vars, got %d: %v", len(env), env)
	}
}

func TestBuildEnvStrictExcludesSecrets(t *testing.T) {
	os.Setenv("TEST_LEAKED_SECRET", "should-not-appear")
	defer os.Unsetenv("TEST_LEAKED_SECRET")

	env := BuildEnv(map[string]string{"ALLOWED": "yes"}, true)
	envMap := envToMap(env)

	if envMap["ALLOWED"] != "yes" {
		t.Error("expected ALLOWED=yes")
	}
	if _, ok := envMap["TEST_LEAKED_SECRET"]; ok {
		t.Error("strict mode should not include parent env vars")
	}
}

func TestBuildEnvStandardInheritsParent(t *testing.T) {
	os.Setenv("TEST_INHERITED_VAR", "visible")
	defer os.Unsetenv("TEST_INHERITED_VAR")

	env := BuildEnv(map[string]string{"EXTRA": "added"}, false)
	envMap := envToMap(env)

	if envMap["TEST_INHERITED_VAR"] != "visible" {
		t.Error("standard mode should inherit parent env vars")
	}
	if envMap["EXTRA"] != "added" {
		t.Error("expected explicit var in standard mode")
	}
}

func TestBuildEnvStrictLimited(t *testing.T) {
	env := BuildEnv(nil, true)
	// Should only have base env (at most 5)
	if len(env) > 5 {
		t.Errorf("strict mode: expected at most 5 vars, got %d", len(env))
	}
}

func TestBuildEnvStandardHasMany(t *testing.T) {
	env := BuildEnv(nil, false)
	// Standard mode inherits full parent — should have many vars
	if len(env) < 5 {
		t.Errorf("standard mode: expected many vars, got %d", len(env))
	}
}

func TestBuildEnvExplicitInBothModes(t *testing.T) {
	explicit := map[string]string{"GITHUB_TOKEN": "resolved-token-value"}

	strictEnv := envToMap(BuildEnv(explicit, true))
	standardEnv := envToMap(BuildEnv(explicit, false))

	if strictEnv["GITHUB_TOKEN"] != "resolved-token-value" {
		t.Error("expected explicit var in strict mode")
	}
	if standardEnv["GITHUB_TOKEN"] != "resolved-token-value" {
		t.Error("expected explicit var in standard mode")
	}
}

func TestConnectStdioRestrictedRestoresEnv(t *testing.T) {
	os.Setenv("TEST_SECRET_TOKEN", "super-secret")
	defer os.Unsetenv("TEST_SECRET_TOKEN")

	envBefore := os.Environ()

	def := MCPServerDef{
		Command:   "false",
		EnvStrict: true,
		Env:       map[string]string{"CUSTOM_VAR": "hello"},
	}
	_, _ = connectStdioRestricted(def)

	envAfter := os.Environ()
	if len(envAfter) != len(envBefore) {
		t.Errorf("env count changed: before=%d after=%d", len(envBefore), len(envAfter))
	}

	if v := os.Getenv("TEST_SECRET_TOKEN"); v != "super-secret" {
		t.Errorf("expected TEST_SECRET_TOKEN restored, got %q", v)
	}
}

func TestConnectStdioRestrictedConcurrentSafe(t *testing.T) {
	os.Setenv("TEST_CONCURRENT_VAR", "original")
	defer os.Unsetenv("TEST_CONCURRENT_VAR")

	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			def := MCPServerDef{Command: "false", EnvStrict: true}
			_, _ = connectStdioRestricted(def)
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

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
