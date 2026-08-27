package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnvWalksUpToTheRepositoryRoot(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "services", "api")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env.local"),
		[]byte("RIDEME_DOTENV_PROBE=found\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Chdir(nested)
	t.Setenv("RIDEME_DOTENV_PROBE", "")
	_ = os.Unsetenv("RIDEME_DOTENV_PROBE")

	LoadDotEnv()

	if got := os.Getenv("RIDEME_DOTENV_PROBE"); got != "found" {
		t.Errorf("RIDEME_DOTENV_PROBE = %q, want %q — the root .env.local was not found from services/api", got, "found")
	}
}

func TestLoadDotEnvIsSilentWhenNoFileExists(t *testing.T) {
	t.Chdir(t.TempDir())
	LoadDotEnv() // must not panic or fail
}
