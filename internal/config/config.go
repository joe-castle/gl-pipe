// Package config handles loading, validating, and persisting gl-pipe's
// YAML configuration, including multi-instance profiles and run presets.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

// Preset is a named, reusable pipeline trigger.
//
// A preset carrying only Variables (the original shape, still valid) is a
// template: <Space> v selects it and the next trigger modal prefills from
// it. A preset that also names Projects is *runnable* — <Space> v fires it
// at those projects on Ref in one keystroke, no modal.
type Preset struct {
	Variables map[string]string `yaml:"variables,omitempty"`

	// Projects are path_with_namespace strings ("backend/api"), resolved
	// against the synced project cache at run time. Paths, not numeric IDs,
	// so the file stays readable and a preset means the same thing when
	// pointed at a different instance; a path that no longer resolves is
	// reported and skipped rather than blocking the rest of the run.
	Projects []string `yaml:"projects,omitempty"`

	// Ref is the branch/tag to trigger on. Empty means "each project's own
	// default branch".
	Ref string `yaml:"ref,omitempty"`
}

// Runnable reports whether this preset names the projects to fire at, and
// so can be triggered directly instead of only prefilling the trigger modal.
func (p Preset) Runnable() bool { return len(p.Projects) > 0 }

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

// SetInstance adds or replaces an instance profile, creating the map on
// first use. The first instance added also becomes the current one, so a
// config built up entirely through the in-app editor is valid without a
// separate "make this active" step.
func (c *Config) SetInstance(name string, inst Instance) {
	if c.Instances == nil {
		c.Instances = map[string]Instance{}
	}
	c.Instances[name] = inst
	if c.CurrentInstance == "" {
		c.CurrentInstance = name
	}
}

// DeleteInstance removes an instance profile. Deleting the active instance
// repoints current_instance at whichever remains (first alphabetically), so
// the config never ends up failing its own Validate; deleting the last
// instance is refused for the same reason.
func (c *Config) DeleteInstance(name string) error {
	if _, ok := c.Instances[name]; !ok {
		return fmt.Errorf("instance %q not found", name)
	}
	if len(c.Instances) == 1 {
		return fmt.Errorf("cannot delete %q: it is the only instance", name)
	}
	delete(c.Instances, name)
	if c.CurrentInstance == name {
		remaining := make([]string, 0, len(c.Instances))
		for n := range c.Instances {
			remaining = append(remaining, n)
		}
		sort.Strings(remaining)
		c.CurrentInstance = remaining[0]
	}
	return nil
}

// SetPreset adds or replaces a named preset, creating the map on first use.
func (c *Config) SetPreset(name string, p Preset) {
	if c.Presets == nil {
		c.Presets = map[string]Preset{}
	}
	c.Presets[name] = p
}

// DeletePreset removes a named preset.
func (c *Config) DeletePreset(name string) { delete(c.Presets, name) }

// PresetNames returns every preset name in sorted order, so the picker and
// settings screen list them in a stable order rather than Go's randomized
// map iteration.
func (c *Config) PresetNames() []string {
	out := make([]string, 0, len(c.Presets))
	for name := range c.Presets {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// InstanceNames returns every instance profile name in sorted order.
func (c *Config) InstanceNames() []string {
	out := make([]string, 0, len(c.Instances))
	for name := range c.Instances {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
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
