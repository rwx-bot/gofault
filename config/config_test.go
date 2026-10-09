package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	content := `
app:
  name: testapp
  environment: testing
  version: 1.0.0
server:
  host: localhost
  port: 8080
database:
  host: 127.0.0.1
  port: 3306
  name: testdb
  user: root
  password: secret
log:
  level: debug
  format: json
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.App.Name != "testapp" {
		t.Errorf("App.Name = %s, want testapp", cfg.App.Name)
	}
	if cfg.App.Environment != "testing" {
		t.Errorf("App.Environment = %s, want testing", cfg.App.Environment)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("Server.Port = %d, want 8080", cfg.Server.Port)
	}
	if cfg.Database.Host != "127.0.0.1" {
		t.Errorf("Database.Host = %s, want 127.0.0.1", cfg.Database.Host)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("Log.Level = %s, want debug", cfg.Log.Level)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("Load() expected error for nonexistent file")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.yaml")
	if err := os.WriteFile(configPath, []byte("invalid: yaml: content:"), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Error("Load() expected error for invalid YAML")
	}
}

func TestMustLoad_Panic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustLoad() should panic on error")
		}
	}()

	MustLoad("/nonexistent/path/config.yaml")
}

func TestServerConfig_GetAddress(t *testing.T) {
	s := ServerConfig{Host: "localhost", Port: 3000}
	if got := s.GetAddress(); got != "localhost:3000" {
		t.Errorf("GetAddress() = %s, want localhost:3000", got)
	}
}

func TestDatabaseConfig_GetDSN(t *testing.T) {
	db := DatabaseConfig{
		Host:     "localhost",
		Port:     3306,
		Name:     "mydb",
		User:     "user",
		Password: "pass",
	}
	expected := "user:pass@tcp(localhost:3306)/mydb?charset=utf8mb4"
	got, err := db.GetDSN()
	if err != nil {
		t.Fatalf("GetDSN() error = %v", err)
	}
	if got != expected {
		t.Errorf("GetDSN() = %s, want %s", got, expected)
	}
}
