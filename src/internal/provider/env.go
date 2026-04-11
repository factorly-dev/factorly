package provider

import "os"

// baseEnv returns a minimal safe environment for child processes.
// Only essential vars are included — no secrets, tokens, or session data.
func baseEnv() []string {
	var env []string
	for _, key := range []string{"PATH", "HOME", "USER", "LANG"} {
		if v, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+v)
		}
	}
	// TERM needed for interactive tools / colored output
	if v, ok := os.LookupEnv("TERM"); ok {
		env = append(env, "TERM="+v)
	}
	return env
}

// buildEnv constructs the environment for a child process:
// base env + explicit key=value pairs + passthrough from parent.
func buildEnv(explicit map[string]string, passthrough []string) []string {
	env := baseEnv()
	for k, v := range explicit {
		env = append(env, k+"="+v)
	}
	for _, k := range passthrough {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	return env
}
