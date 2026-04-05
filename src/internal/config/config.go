package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Verbose is an optional log function for debug output. Set by the CLI --verbose flag.
var Verbose func(format string, args ...any)

func vlog(format string, args ...any) {
	if Verbose != nil {
		Verbose(format, args...)
	}
}

type Config struct {
	ToolsDir string                `yaml:"tools_dir,omitempty"`
	Tools    map[string]ToolConfig `yaml:"tools"`
}

type ToolConfig struct {
	Type        string            `yaml:"type"`
	Description string            `yaml:"description"`
	Command     string            `yaml:"command,omitempty"`
	Args        []string          `yaml:"args,omitempty"`
	Stdin       string            `yaml:"stdin,omitempty"`
	Env         map[string]string `yaml:"env,omitempty"`
	Parameters  []ParamConfig     `yaml:"parameters,omitempty"`

	// MCP fields
	URL string `yaml:"url,omitempty"` // MCP HTTP transport URL

	// REST fields
	BaseURL string            `yaml:"base_url,omitempty"`
	Method  string            `yaml:"method,omitempty"`
	Path    string            `yaml:"path,omitempty"`
	Headers map[string]string `yaml:"headers,omitempty"`
	Auth    *AuthConfig       `yaml:"auth,omitempty"`
}

type AuthConfig struct {
	Type   string `yaml:"type"`             // "bearer", "basic", "header"
	Token  string `yaml:"token,omitempty"`  // for bearer
	Header string `yaml:"header,omitempty"` // for header-based auth
	Value  string `yaml:"value,omitempty"`  // for header-based auth
}

type ParamConfig struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
	Required    bool   `yaml:"required,omitempty"`
	In          string `yaml:"in,omitempty"` // "query", "path", "header", "body"
}

var envVarPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)
var placeholderPattern = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_-]*)\}`)

func Load(path string) (*Config, error) {
	if path == "" {
		path = "factorly.yaml"
	}

	var cfg Config

	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("reading config: %w", err)
		}
		vlog("config file not found: %s (will check .factorly/)", path)
		cfg.Tools = make(map[string]ToolConfig)
	} else {
		vlog("loaded config: %s", path)
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("parsing config: %w", err)
		}
		if cfg.Tools == nil {
			cfg.Tools = make(map[string]ToolConfig)
		}
	}

	// Load tools from tools_dir if set
	if cfg.ToolsDir != "" {
		dir := cfg.ToolsDir
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(filepath.Dir(path), dir)
		}
		vlog("loading tools_dir: %s", dir)
		dirTools, err := loadDir(dir)
		if err != nil {
			return nil, err
		}
		vlog("  found %d tools in tools_dir", len(dirTools))
		if err := mergeTools(cfg.Tools, dirTools); err != nil {
			return nil, err
		}
	}

	// Merge .factorly/ project directory if it exists.
	configDir := filepath.Dir(path)
	if filepath.Base(configDir) == ".factorly" {
		vlog("loading loose tool files from %s", configDir)
		dirTools, err := loadDir(configDir)
		if err != nil {
			return nil, err
		}
		if len(dirTools) > 0 {
			vlog("  found %d tools in %s", len(dirTools), configDir)
		}
		if err := mergeTools(cfg.Tools, dirTools); err != nil {
			return nil, err
		}
	} else {
		// Config is outside .factorly/ — check for a .factorly/ subdirectory
		if err := mergeProjectDir(&cfg, configDir); err != nil {
			return nil, err
		}
	}

	resolveEnvVars(&cfg)
	inferParameters(&cfg)

	if err := validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// LoadDir loads tool definitions from a directory of YAML files (no primary config file).
func LoadDir(dirPath string) (*Config, error) {
	tools, err := loadDir(dirPath)
	if err != nil {
		return nil, err
	}

	cfg := &Config{Tools: tools}
	resolveEnvVars(cfg)
	inferParameters(cfg)

	if err := validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func loadDir(dir string) (map[string]ToolConfig, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading tools_dir %q: %w", dir, err)
	}

	// Sort for deterministic load order
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	all := make(map[string]ToolConfig)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := filepath.Ext(name)
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		// Skip the primary config file if present in the same directory
		if name == "factorly.yaml" || name == "factorly.yml" {
			continue
		}

		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}

		var tools map[string]ToolConfig
		if err := yaml.Unmarshal(data, &tools); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", path, err)
		}

		for toolName, toolCfg := range tools {
			if _, exists := all[toolName]; exists {
				return nil, fmt.Errorf("config: duplicate tool %q (found again in %s)", toolName, name)
			}
			all[toolName] = toolCfg
		}
	}

	return all, nil
}

func mergeTools(base, extras map[string]ToolConfig) error {
	for name, tool := range extras {
		if _, exists := base[name]; exists {
			return fmt.Errorf("config: duplicate tool %q (defined in both factorly.yaml and tools_dir)", name)
		}
		base[name] = tool
	}
	return nil
}

// mergeProjectDir merges tool definitions from a .factorly/ directory
// relative to baseDir. It loads .factorly/factorly.yaml if present
// (merging its tools and following its tools_dir), and also loads any
// loose YAML tool files directly in .factorly/.
func mergeProjectDir(cfg *Config, baseDir string) error {
	projectDir := filepath.Join(baseDir, ".factorly")
	info, err := os.Stat(projectDir)
	if err != nil || !info.IsDir() {
		return nil
	}

	vlog("found project directory: %s", projectDir)

	// Check for .factorly/factorly.yaml
	projectConfig := filepath.Join(projectDir, "factorly.yaml")
	if _, err := os.Stat(projectConfig); err == nil {
		vlog("loading project config: %s", projectConfig)
		data, err := os.ReadFile(projectConfig)
		if err != nil {
			return fmt.Errorf("reading %s: %w", projectConfig, err)
		}
		var projectCfg Config
		if err := yaml.Unmarshal(data, &projectCfg); err != nil {
			return fmt.Errorf("parsing %s: %w", projectConfig, err)
		}

		// Follow tools_dir from project config
		if projectCfg.ToolsDir != "" {
			dir := projectCfg.ToolsDir
			if !filepath.IsAbs(dir) {
				dir = filepath.Join(projectDir, dir)
			}
			dirTools, err := loadDir(dir)
			if err != nil {
				return err
			}
			if projectCfg.Tools == nil {
				projectCfg.Tools = make(map[string]ToolConfig)
			}
			if err := mergeTools(projectCfg.Tools, dirTools); err != nil {
				return err
			}
		}

		// Merge project tools into main config
		if err := mergeTools(cfg.Tools, projectCfg.Tools); err != nil {
			return err
		}
	}

	// Also load loose YAML files in .factorly/ (loadDir skips factorly.yaml)
	dirTools, err := loadDir(projectDir)
	if err != nil {
		return err
	}
	if len(dirTools) > 0 {
		if err := mergeTools(cfg.Tools, dirTools); err != nil {
			return err
		}
	}
	return nil
}

func resolveEnvVars(cfg *Config) {
	for name, tool := range cfg.Tools {
		tool.Command = resolveString(tool.Command)
		tool.Description = resolveString(tool.Description)
		tool.Stdin = resolveString(tool.Stdin)
		for i, arg := range tool.Args {
			tool.Args[i] = resolveString(arg)
		}
		for k, v := range tool.Env {
			tool.Env[k] = resolveString(v)
		}
		// MCP fields
		tool.URL = resolveString(tool.URL)
		// REST fields
		tool.BaseURL = resolveString(tool.BaseURL)
		tool.Path = resolveString(tool.Path)
		for k, v := range tool.Headers {
			tool.Headers[k] = resolveString(v)
		}
		if tool.Auth != nil {
			tool.Auth.Token = resolveString(tool.Auth.Token)
			tool.Auth.Value = resolveString(tool.Auth.Value)
		}
		cfg.Tools[name] = tool
	}
}

func resolveString(s string) string {
	return envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		varName := envVarPattern.FindStringSubmatch(match)[1]
		if val, ok := os.LookupEnv(varName); ok {
			return val
		}
		return match
	})
}

func inferParameters(cfg *Config) {
	for name, tool := range cfg.Tools {
		if len(tool.Parameters) > 0 {
			continue
		}
		seen := make(map[string]bool)
		// Infer from args
		for _, arg := range tool.Args {
			matches := placeholderPattern.FindAllStringSubmatch(arg, -1)
			for _, m := range matches {
				paramName := m[1]
				if !seen[paramName] {
					seen[paramName] = true
					tool.Parameters = append(tool.Parameters, ParamConfig{
						Name:     paramName,
						Required: true,
					})
				}
			}
		}
		// Infer from stdin
		if tool.Stdin != "" {
			matches := placeholderPattern.FindAllStringSubmatch(tool.Stdin, -1)
			for _, m := range matches {
				paramName := m[1]
				if !seen[paramName] {
					seen[paramName] = true
					tool.Parameters = append(tool.Parameters, ParamConfig{
						Name:     paramName,
						Required: true,
					})
				}
			}
		}
		cfg.Tools[name] = tool
	}
}

func validate(cfg *Config) error {
	if len(cfg.Tools) == 0 {
		return fmt.Errorf("config: no tools defined")
	}
	validTypes := map[string]bool{"cli": true, "mcp": true, "rest": true}
	for name, tool := range cfg.Tools {
		if tool.Type == "" {
			return fmt.Errorf("config: tool %q missing type", name)
		}
		if !validTypes[tool.Type] {
			return fmt.Errorf("config: tool %q has unknown type %q", name, tool.Type)
		}
		if tool.Type == "cli" && tool.Command == "" {
			return fmt.Errorf("config: cli tool %q missing command", name)
		}
		if tool.Type == "mcp" {
			if tool.Command == "" && tool.URL == "" {
				return fmt.Errorf("config: mcp tool %q needs either command (stdio) or url (http)", name)
			}
		}
		if tool.Type == "rest" {
			if tool.BaseURL == "" {
				return fmt.Errorf("config: rest tool %q missing base_url", name)
			}
			if tool.Method == "" {
				return fmt.Errorf("config: rest tool %q missing method", name)
			}
		}
	}
	return nil
}

func FindConfig() string {
	// Prefer top-level config — .factorly/ merges into it via Load()
	candidates := []string{
		"factorly.yaml",
		"factorly.yml",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	// No top-level config — check .factorly/ project directory
	if _, err := os.Stat(".factorly/factorly.yaml"); err == nil {
		return ".factorly/factorly.yaml"
	}
	// .factorly/ exists with loose files but no factorly.yaml inside
	if info, err := os.Stat(".factorly"); err == nil && info.IsDir() {
		return ".factorly/factorly.yaml"
	}
	if home, err := os.UserHomeDir(); err == nil {
		p := home + "/.config/factorly/factorly.yaml"
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "factorly.yaml"
}

// ParamNames returns the list of parameter names for a tool config.
func (tc *ToolConfig) ParamNames() []string {
	names := make([]string, len(tc.Parameters))
	for i, p := range tc.Parameters {
		names[i] = p.Name
	}
	return names
}

// HasPlaceholder checks if any arg contains {name} style placeholders.
func HasPlaceholder(args []string, name string) bool {
	target := "{" + name + "}"
	for _, arg := range args {
		if strings.Contains(arg, target) {
			return true
		}
	}
	return false
}
