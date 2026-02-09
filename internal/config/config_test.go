package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Claude.Command != "claude --continue --permission-mode acceptEdits" {
		t.Errorf("Expected default claude command, got %q", cfg.Claude.Command)
	}
	if cfg.Claude.Key != "c" {
		t.Errorf("Expected default key 'c', got %q", cfg.Claude.Key)
	}
	if !cfg.Claude.Enabled {
		t.Error("Claude should be enabled by default")
	}
	if cfg.Codex.Command != "codex" {
		t.Errorf("Expected default codex command, got %q", cfg.Codex.Command)
	}
	if cfg.Codex.Key != "x" {
		t.Errorf("Expected default codex key 'x', got %q", cfg.Codex.Key)
	}
	if !cfg.Codex.Enabled {
		t.Error("Codex should be enabled by default")
	}
	if len(cfg.Sessions) != 0 {
		t.Errorf("Expected no custom sessions by default, got %d", len(cfg.Sessions))
	}
}

func TestLoadDefaultWhenNoFile(t *testing.T) {
	// Mock config path to non-existent location
	origHome := os.Getenv("HOME")
	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load should not error when file doesn't exist: %v", err)
	}

	if cfg.Claude.Command != "claude --continue --permission-mode acceptEdits" {
		t.Error("Should return default config when file doesn't exist")
	}
	if cfg.Codex.Command != "codex" || cfg.Codex.Key != "x" || !cfg.Codex.Enabled {
		t.Error("Should include default codex config when file doesn't exist")
	}
}

func TestLoadValidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".config", "pocketbot")
	os.MkdirAll(configDir, 0755)

	configContent := `
claude:
  command: "claude --continue"
  key: "c"
  enabled: true

codex:
  command: "codex --model gpt-5"
  key: "x"
  enabled: true

sessions:
  - name: "dev-server"
    command: "npm run dev"
    key: "d"
  - name: "api"
    command: "go run main.go"
    key: "a"
`
	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if len(cfg.Sessions) != 2 {
		t.Errorf("Expected 2 sessions, got %d", len(cfg.Sessions))
	}
	if cfg.Codex.Command != "codex --model gpt-5" {
		t.Errorf("Expected codex command to be loaded, got %q", cfg.Codex.Command)
	}
	if cfg.Codex.Key != "x" {
		t.Errorf("Expected codex key 'x', got %q", cfg.Codex.Key)
	}

	if cfg.Sessions[0].Name != "dev-server" {
		t.Errorf("Expected session name 'dev-server', got %q", cfg.Sessions[0].Name)
	}
	if cfg.Sessions[0].Key != "d" {
		t.Errorf("Expected key 'd', got %q", cfg.Sessions[0].Key)
	}
}

func TestLoadValidConfigCodexDisabled(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".config", "pocketbot")
	os.MkdirAll(configDir, 0755)

	configContent := `
claude:
  command: "claude --continue"
  key: "c"
  enabled: true

codex:
  command: "codex"
  key: "x"
  enabled: false
`
	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.Codex.Enabled {
		t.Error("Expected codex to remain disabled when explicitly set to false")
	}
}

func TestValidateDuplicateKeys(t *testing.T) {
	cfg := &Config{
		Claude: ClaudeConfig{
			Command: "claude --continue",
			Key:     "c",
			Enabled: true,
		},
		Codex: CodexConfig{
			Command: "codex",
			Key:     "x",
			Enabled: true,
		},
		Sessions: []SessionConfig{
			{Name: "test", Command: "echo test", Key: "x"}, // Duplicate key with codex
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("Expected validation error for duplicate keys")
	}
}

func TestValidateMissingFields(t *testing.T) {
	tests := []struct {
		name    string
		session SessionConfig
		wantErr bool
	}{
		{
			name:    "missing name",
			session: SessionConfig{Command: "test", Key: "t"},
			wantErr: true,
		},
		{
			name:    "missing command",
			session: SessionConfig{Name: "test", Key: "t"},
			wantErr: true,
		},
		{
			name:    "missing key",
			session: SessionConfig{Name: "test", Command: "test"},
			wantErr: true,
		},
		{
			name:    "all fields present",
			session: SessionConfig{Name: "test", Command: "test", Key: "t"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Claude:   ClaudeConfig{Enabled: false},
				Codex:    CodexConfig{Enabled: false},
				Sessions: []SessionConfig{tt.session},
			}
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAllSessions(t *testing.T) {
	cfg := &Config{
		Claude: ClaudeConfig{
			Command: "claude --continue",
			Key:     "c",
			Enabled: true,
		},
		Codex: CodexConfig{
			Command: "codex",
			Key:     "x",
			Enabled: true,
		},
		Sessions: []SessionConfig{
			{Name: "test1", Command: "test1", Key: "t"},
			{Name: "test2", Command: "test2", Key: "u"},
		},
	}

	all := cfg.AllSessions()
	if len(all) != 4 {
		t.Errorf("Expected 4 sessions (claude + codex + 2 custom), got %d", len(all))
	}

	if all[0].Name != "claude" {
		t.Error("First session should be claude")
	}
	if all[1].Name != "codex" {
		t.Error("Second session should be codex")
	}
}

func TestAllSessionsClaudeDisabled(t *testing.T) {
	cfg := &Config{
		Claude: ClaudeConfig{
			Command: "claude --continue",
			Key:     "c",
			Enabled: false,
		},
		Codex: CodexConfig{
			Command: "codex",
			Key:     "x",
			Enabled: false,
		},
		Sessions: []SessionConfig{
			{Name: "test1", Command: "test1", Key: "t"},
		},
	}

	all := cfg.AllSessions()
	if len(all) != 1 {
		t.Errorf("Expected 1 session (claude disabled), got %d", len(all))
	}

	if all[0].Name != "test1" {
		t.Error("Should not include claude when disabled")
	}
}
