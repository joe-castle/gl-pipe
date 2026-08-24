package config

import (
	"runtime"
	"testing"
)

func TestResolveToken_Literal(t *testing.T) {
	inst := Instance{Token: "glpat-literal-value"}
	got, err := inst.ResolveToken()
	if err != nil {
		t.Fatalf("ResolveToken returned error: %v", err)
	}
	if got != "glpat-literal-value" {
		t.Errorf("ResolveToken() = %q, want literal token", got)
	}
}

func TestResolveToken_EnvVarExpansion(t *testing.T) {
	t.Setenv("GL_PIPE_TEST_TOKEN", "glpat-from-env")
	inst := Instance{Token: "${GL_PIPE_TEST_TOKEN}"}

	got, err := inst.ResolveToken()
	if err != nil {
		t.Fatalf("ResolveToken returned error: %v", err)
	}
	if got != "glpat-from-env" {
		t.Errorf("ResolveToken() = %q, want value from env", got)
	}
}

func TestResolveToken_EnvVarMissing(t *testing.T) {
	inst := Instance{Token: "${GL_PIPE_TEST_TOKEN_DOES_NOT_EXIST}"}

	if _, err := inst.ResolveToken(); err == nil {
		t.Fatal("expected error for unset env var, got nil")
	}
}

func TestResolveToken_NoTokenConfigured(t *testing.T) {
	inst := Instance{}
	if _, err := inst.ResolveToken(); err == nil {
		t.Fatal("expected error when no token or token_command set, got nil")
	}
}

func TestResolveToken_Command(t *testing.T) {
	var cmdline string
	if runtime.GOOS == "windows" {
		cmdline = "cmd /C echo glpat-from-command"
	} else {
		cmdline = "echo glpat-from-command"
	}
	inst := Instance{TokenCommand: cmdline}

	got, err := inst.ResolveToken()
	if err != nil {
		t.Fatalf("ResolveToken returned error: %v", err)
	}
	if got != "glpat-from-command" {
		t.Errorf("ResolveToken() = %q, want %q", got, "glpat-from-command")
	}
}

func TestResolveToken_CommandTakesPrecedenceOverToken(t *testing.T) {
	var cmdline string
	if runtime.GOOS == "windows" {
		cmdline = "cmd /C echo from-command"
	} else {
		cmdline = "echo from-command"
	}
	inst := Instance{Token: "from-literal", TokenCommand: cmdline}

	got, err := inst.ResolveToken()
	if err != nil {
		t.Fatalf("ResolveToken returned error: %v", err)
	}
	if got != "from-command" {
		t.Errorf("ResolveToken() = %q, want token_command output to win", got)
	}
}

func TestResolveToken_CommandFailureIsWrapped(t *testing.T) {
	inst := Instance{TokenCommand: "does-not-exist-binary-xyz"}
	if _, err := inst.ResolveToken(); err == nil {
		t.Fatal("expected error for nonexistent command, got nil")
	}
}
