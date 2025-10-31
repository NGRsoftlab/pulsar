package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewLoader(t *testing.T) {
	loader := NewLoader()

	if loader == nil {
		t.Fatal("NewLoader returned nil")
	}

	if loader.config == nil {
		t.Fatal("Loader config is nil")
	}

	// Проверяем что загружены дефолты
	if loader.config.Generator.EventsPerSecond != 10 {
		t.Errorf("Expected default events per second 10, got %d", loader.config.Generator.EventsPerSecond)
	}
}

func TestLoadFromFile(t *testing.T) {
	// Создаем временный конфиг файл
	configData := `
generator:
  name: "test-generator"
  events_per_second: 50
  event_types: ["netflow", "syslog"]
  duration: "30s"

sender:
  name: "test-sender"  
  protocol: "tcp"
  destinations:
    - "10.0.1.100:514"
    - "10.0.1.101:514"

pipeline:
  buffer_size: 200
  worker_count: 2

logging:
  level: "debug"
  format: "json"
`

	tmpDir, err := os.MkdirTemp("", "config-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configFile := filepath.Join(tmpDir, "test.yaml")
	err = os.WriteFile(configFile, []byte(configData), 0o644)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	loader := NewLoader()
	err = loader.LoadFromFile(configFile)
	if err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}

	cfg := loader.GetConfig()

	// Проверяем загруженные значения
	if cfg.Generator.EventsPerSecond != 50 {
		t.Errorf("Expected events per second 50, got %d", cfg.Generator.EventsPerSecond)
	}

	if len(cfg.Generator.EventTypes) != 2 {
		t.Errorf("Expected 2 event types, got %d", len(cfg.Generator.EventTypes))
	}

	if cfg.Generator.Duration != 30*time.Second {
		t.Errorf("Expected duration 30s, got %v", cfg.Generator.Duration)
	}

	if cfg.Sender.Protocol != "tcp" {
		t.Errorf("Expected protocol 'tcp', got '%s'", cfg.Sender.Protocol)
	}

	if len(cfg.Sender.Destinations) != 2 {
		t.Errorf("Expected 2 destinations, got %d", len(cfg.Sender.Destinations))
	}

	if cfg.Pipeline.BufferSize != 200 {
		t.Errorf("Expected buffer size 200, got %d", cfg.Pipeline.BufferSize)
	}

	if cfg.Logging.Level != "debug" {
		t.Errorf("Expected log level 'debug', got '%s'", cfg.Logging.Level)
	}
}

func TestLoadFromFile_NotFound(t *testing.T) {
	loader := NewLoader()
	err := loader.LoadFromFile("nonexistent.yaml")

	if err == nil {
		t.Error("Expected error for nonexistent file, got nil")
	}

	if !contains(err.Error(), "not found") {
		t.Errorf("Expected 'not found' error, got: %v", err)
	}
}

func TestLoadFromEnv(t *testing.T) {
	// Сохраняем оригинальные значения
	originalValues := make(map[string]string)
	envVars := []string{
		"UEBA_GENERATOR_NAME",
		"UEBA_EVENTS_PER_SEC",
		"UEBA_EVENT_TYPES",
		"UEBA_PROTOCOL",
		"UEBA_DESTINATIONS",
		"UEBA_BUFFER_SIZE",
		"UEBA_LOG_LEVEL",
		"UEBA_DURATION",
	}

	for _, env := range envVars {
		originalValues[env] = os.Getenv(env)
	}

	// Восстанавливаем после теста
	defer func() {
		for _, env := range envVars {
			if val, exists := originalValues[env]; exists && val != "" {
				os.Setenv(env, val)
			} else {
				os.Unsetenv(env)
			}
		}
	}()

	// Устанавливаем тестовые значения
	os.Setenv("UEBA_GENERATOR_NAME", "env-generator")
	os.Setenv("UEBA_EVENTS_PER_SEC", "123")
	os.Setenv("UEBA_EVENT_TYPES", "netflow,syslog")
	os.Setenv("UEBA_PROTOCOL", "tcp")
	os.Setenv("UEBA_DESTINATIONS", "1.1.1.1:514,2.2.2.2:514")
	os.Setenv("UEBA_BUFFER_SIZE", "300")
	os.Setenv("UEBA_LOG_LEVEL", "debug")
	os.Setenv("UEBA_DURATION", "1h30m")

	loader := NewLoader()
	err := loader.LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv failed: %v", err)
	}

	cfg := loader.GetConfig()

	// Проверяем загруженные значения
	if cfg.Generator.EventsPerSecond != 123 {
		t.Errorf("Expected events per second 123, got %d", cfg.Generator.EventsPerSecond)
	}

	if len(cfg.Generator.EventTypes) != 2 {
		t.Errorf("Expected 2 event types, got %d", len(cfg.Generator.EventTypes))
	}

	if cfg.Sender.Protocol != "tcp" {
		t.Errorf("Expected protocol 'tcp', got '%s'", cfg.Sender.Protocol)
	}

	if len(cfg.Sender.Destinations) != 2 {
		t.Errorf("Expected 2 destinations, got %d", len(cfg.Sender.Destinations))
	}

	if cfg.Pipeline.BufferSize != 300 {
		t.Errorf("Expected buffer size 300, got %d", cfg.Pipeline.BufferSize)
	}

	if cfg.Logging.Level != "debug" {
		t.Errorf("Expected log level 'debug', got '%s'", cfg.Logging.Level)
	}

	expectedDuration := 1*time.Hour + 30*time.Minute
	if cfg.Generator.Duration != expectedDuration {
		t.Errorf("Expected duration %v, got %v", expectedDuration, cfg.Generator.Duration)
	}
}

func TestApplyFlags(t *testing.T) {
	loader := NewLoader()

	flags := &Flags{
		Rate:         999,
		Destinations: []string{"flag1:514", "flag2:514"},
		Protocol:     "tcp",
		BufferSize:   500,
		LogLevel:     "error",
		Duration:     45 * time.Minute,
	}

	loader.ApplyFlags(flags)
	cfg := loader.GetConfig()

	// Проверяем что флаги применились
	if cfg.Generator.EventsPerSecond != 999 {
		t.Errorf("Expected events per second 999, got %d", cfg.Generator.EventsPerSecond)
	}

	if len(cfg.Sender.Destinations) != 2 {
		t.Errorf("Expected 2 destinations, got %d", len(cfg.Sender.Destinations))
	}

	if cfg.Sender.Destinations[0] != "flag1:514" {
		t.Errorf("Expected first destination 'flag1:514', got '%s'", cfg.Sender.Destinations[0])
	}

	if cfg.Sender.Protocol != "tcp" {
		t.Errorf("Expected protocol 'tcp', got '%s'", cfg.Sender.Protocol)
	}

	if cfg.Pipeline.BufferSize != 500 {
		t.Errorf("Expected buffer size 500, got %d", cfg.Pipeline.BufferSize)
	}

	if cfg.Logging.Level != "error" {
		t.Errorf("Expected log level 'error', got '%s'", cfg.Logging.Level)
	}

	if cfg.Generator.Duration != 45*time.Minute {
		t.Errorf("Expected duration 45m, got %v", cfg.Generator.Duration)
	}
}

func TestLoadConfig_Integration(t *testing.T) {
	// Тест полного цикла загрузки конфигурации
	flags := &Flags{
		Rate:     100,
		Protocol: "udp",
		LogLevel: "info",
	}

	cfg, err := LoadConfig("", flags)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Проверяем что конфигурация валидная
	if err := cfg.Validate(); err != nil {
		t.Errorf("Configuration validation failed: %v", err)
	}

	// Проверяем что флаги применились
	if cfg.Generator.EventsPerSecond != 100 {
		t.Errorf("Expected events per second 100, got %d", cfg.Generator.EventsPerSecond)
	}

	if cfg.Sender.Protocol != "udp" {
		t.Errorf("Expected protocol 'udp', got '%s'", cfg.Sender.Protocol)
	}
}
