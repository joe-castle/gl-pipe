package config

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

var envVarPattern = regexp.MustCompile(`^\$\{([A-Za-z_][A-Za-z0-9_]*)\}$`)

// ResolveToken returns the instance's personal access token. Precedence:
// TokenCommand (shell out, e.g. to a password manager) beats Token; a Token
// of the form "${ENV_VAR}" is expanded from the environment; anything else
// in Token is used literally.
func (i Instance) ResolveToken() (string, error) {
	if i.TokenCommand != "" {
		return runTokenCommand(i.TokenCommand)
	}
	if i.Token == "" {
		return "", fmt.Errorf("instance has no token or token_command configured")
	}
	if m := envVarPattern.FindStringSubmatch(strings.TrimSpace(i.Token)); m != nil {
		val, ok := os.LookupEnv(m[1])
		if !ok || val == "" {
			return "", fmt.Errorf("environment variable %s is not set", m[1])
		}
		return val, nil
	}
	return i.Token, nil
}

func runTokenCommand(cmdline string) (string, error) {
	parts := strings.Fields(cmdline)
	if len(parts) == 0 {
		return "", fmt.Errorf("token_command is empty")
	}
	cmd := exec.Command(parts[0], parts[1:]...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("token_command %q failed: %w", cmdline, err)
	}
	token := strings.TrimSpace(string(out))
	if token == "" {
		return "", fmt.Errorf("token_command %q produced empty output", cmdline)
	}
	return token, nil
}
