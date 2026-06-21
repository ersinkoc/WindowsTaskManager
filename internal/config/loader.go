package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Load reads and parses a YAML config file. If the file does not exist,
// a default config is created.
func Load(path string) (*Config, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		cfg := DefaultConfig()
		if err := Save(path, cfg); err != nil {
			return nil, fmt.Errorf("create default config: %w", err)
		}
		return cfg, nil
	}

	// #nosec G304 -- path is sourced from local app config path (not remote input).
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	// Schema migration. When we ship a breaking defaults change we bump
	// CurrentSchemaVersion; on load we stamp in the new anomaly block so
	// existing configs actually see the new defaults. We preserve the
	// user's ignore list customisations since those are additive.
	if cfg.SchemaVersion < CurrentSchemaVersion {
		preservedIgnore := cfg.Anomaly.IgnoreProcesses
		defaults := DefaultConfig()
		cfg.Anomaly = defaults.Anomaly
		if len(preservedIgnore) > 0 {
			cfg.Anomaly.IgnoreProcesses = mergeUniqueFold(defaults.Anomaly.IgnoreProcesses, preservedIgnore)
		}
		cfg.SchemaVersion = CurrentSchemaVersion
		if err := Save(path, cfg); err != nil {
			return nil, fmt.Errorf("persist migrated config: %w", err)
		}
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}
	return cfg, nil
}

func mergeUniqueFold(base, extra []string) []string {
	seen := make(map[string]struct{}, len(base)+len(extra))
	out := make([]string, 0, len(base)+len(extra))
	for _, x := range base {
		k := strings.ToLower(strings.TrimSpace(x))
		if k == "" {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, x)
	}
	for _, x := range extra {
		k := strings.ToLower(strings.TrimSpace(x))
		if k == "" {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, x)
	}
	return out
}

// yamlMarshal is the function used to serialize a *Config to YAML. It is a
// package-level variable so tests can substitute a stub that returns an
// error to exercise the error path in Save, which is otherwise unreachable
// for a well-formed *Config value.
var yamlMarshal = yaml.Marshal

// Save writes the config as YAML to the given path.
func Save(path string, cfg *Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	data, err := yamlMarshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}

// ParseSize parses a human-readable size such as "10MB", "2GB", "512KB", or "10MB/min".
// Returns the value in bytes (or bytes per minute when /min suffix is used).
func ParseSize(s string) uint64 {
	s = strings.TrimSpace(strings.ToUpper(s))
	if s == "" {
		return 0
	}
	// Strip "/MIN" suffix if present.
	s = strings.TrimSuffix(s, "/MIN")

	multipliers := []struct {
		suffix string
		mult   uint64
	}{
		{"TB", 1 << 40},
		{"GB", 1 << 30},
		{"MB", 1 << 20},
		{"KB", 1 << 10},
		{"B", 1},
	}

	for _, m := range multipliers {
		if strings.HasSuffix(s, m.suffix) {
			n := strings.TrimSuffix(s, m.suffix)
			n = strings.TrimSpace(n)
			val, err := strconv.ParseFloat(n, 64)
			if err != nil {
				return 0
			}
			return uint64(val * float64(m.mult))
		}
	}
	val, _ := strconv.ParseUint(s, 10, 64)
	return val
}
