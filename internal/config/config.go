// Package config handles loading, validating, and persisting gl-pipe's
// YAML configuration, including multi-instance profiles and run presets.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	appDirName     = "gl-pipe"
	configFileName = "config.yaml"
)

// Config is the root gl-pipe configuration document.
type Config struct {
	CurrentInstance string              `yaml:"current_instance"`
	Instances       map[string]Instance `yaml:"instances"`
	Cache           CacheConfig         `yaml:"cache"`
	Pipelines       PipelinesConfig     `yaml:"pipelines,omitempty"`
	Presets         map[string]Preset   `yaml:"presets,omitempty"`
}

// Instance is one GitLab profile (e.g. "work" or "personal").
type Instance struct {
	URL           string   `yaml:"url"`
	Token         string   `yaml:"token,omitempty"`
	TokenCommand  string   `yaml:"token_command,omitempty"`
	DefaultGroups []string `yaml:"default_groups,omitempty"`
}

// CacheConfig controls the local project index TTL.
type CacheConfig struct {
	TTLMinutes int `yaml:"ttl_minutes"`
}

// PipelinesConfig controls how broad pipeline queries (viewing staged
// projects' pipelines, a ref search) are scoped.
type PipelinesConfig struct {
	// MaxAgeDays caps how far back a pipeline query looks (GitLab's
	// created_after filter, applied server-side — fewer results over the
	// wire, not just a client-side hide). 0/unset means no cap, the
	// pre-existing default: only each request's own page-size limit
	// applies, same as before this existed.
	MaxAgeDays int `yaml:"max_age_days,omitempty"`
}

// Preset is a named bundle of pipeline trigger variables.
type Preset struct {
	Variables map[string]string `yaml:"variables"`
}

// DefaultPath returns the OS-appropriate config file location:
// ~/.config/gl-pipe/config.yaml or %APPDATA%\gl-pipe\config.yaml.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolving user config dir: %w", err)
	}
	return filepath.Join(dir, appDirName, configFileName), nil
}

// Exists reports whether a config file is present at path.
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Load reads, parses, and validates the config file at path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Save writes the config to path with owner-only permissions, creating
// parent directories as needed.
func (c *Config) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return nil
}

// Validate checks structural invariants required before the config can be used.
func (c *Config) Validate() error {
	if len(c.Instances) == 0 {
		return fmt.Errorf("config has no instances defined")
	}
	for name, inst := range c.Instances {
		if inst.URL == "" {
			return fmt.Errorf("instance %q has no url", name)
		}
	}
	if c.CurrentInstance == "" {
		return fmt.Errorf("current_instance is not set")
	}
	if _, ok := c.Instances[c.CurrentInstance]; !ok {
		return fmt.Errorf("current_instance %q is not defined in instances", c.CurrentInstance)
	}
	return nil
}

// Active returns the currently selected instance.
func (c *Config) Active() (Instance, error) {
	inst, ok := c.Instances[c.CurrentInstance]
	if !ok {
		return Instance{}, fmt.Errorf("current instance %q not found", c.CurrentInstance)
	}
	return inst, nil
}

// SetActive switches the current instance profile.
func (c *Config) SetActive(name string) error {
	if _, ok := c.Instances[name]; !ok {
		return fmt.Errorf("instance %q not found", name)
	}
	c.CurrentInstance = name
	return nil
}

// TTL returns the cache TTL, defaulting to 60 minutes when unset.
func (c *Config) TTL() time.Duration {
	if c.Cache.TTLMinutes <= 0 {
		return 60 * time.Minute
	}
	return time.Duration(c.Cache.TTLMinutes) * time.Minute
}

// PipelineMaxAge returns how far back pipeline queries should look, or 0
// (no cap) if unset.
func (c *Config) PipelineMaxAge() time.Duration {
	if c.Pipelines.MaxAgeDays <= 0 {
		return 0
	}
	return time.Duration(c.Pipelines.MaxAgeDays) * 24 * time.Hour
}
