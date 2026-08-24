// Command gl-pipe is a keyboard-first terminal UI for managing GitLab
// CI/CD pipelines across multiple repositories at once.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alecthomas/kong"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/joeca/gl-pipe/internal/config"
	"github.com/joeca/gl-pipe/internal/ui"
)

// CLI holds the flags that can override the active config profile at
// runtime, per the spec's --url / --token / --config / --profile.
type CLI struct {
	URL     string `help:"GitLab instance URL, overrides the active profile for this run." optional:""`
	Token   string `help:"Personal access token, overrides the active profile for this run." optional:""`
	Config  string `help:"Path to config.yaml (default: OS config dir)." optional:"" type:"path"`
	Profile string `help:"Instance profile to activate for this run." optional:""`
}

func main() {
	var cli CLI
	kong.Parse(&cli,
		kong.Name("gl-pipe"),
		kong.Description("Manage, search, inspect, and batch-trigger GitLab CI/CD pipelines across multiple repositories."),
	)

	configPath := cli.Config
	if configPath == "" {
		p, err := config.DefaultPath()
		if err != nil {
			fatal(err)
		}
		configPath = p
	}

	var cfg *config.Config
	if config.Exists(configPath) {
		loaded, err := config.Load(configPath)
		if err != nil {
			fatal(fmt.Errorf("loading config: %w", err))
		}
		cfg = loaded
		applyOverrides(cfg, cli)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := ui.New(ctx, cancel, cfg, configPath, filepath.Dir(configPath))

	program := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		fatal(err)
	}
}

func applyOverrides(cfg *config.Config, cli CLI) {
	if cli.Profile != "" {
		if err := cfg.SetActive(cli.Profile); err != nil {
			fatal(err)
		}
	}
	if cli.URL == "" && cli.Token == "" {
		return
	}
	inst, err := cfg.Active()
	if err != nil {
		fatal(err)
	}
	if cli.URL != "" {
		inst.URL = cli.URL
	}
	if cli.Token != "" {
		inst.Token = cli.Token
		inst.TokenCommand = ""
	}
	cfg.Instances[cfg.CurrentInstance] = inst
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "gl-pipe:", err)
	os.Exit(1)
}
