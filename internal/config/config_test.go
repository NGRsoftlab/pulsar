package config

import (
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func withEnv(t *testing.T, key, value string, fn func()) {
	t.Helper()
	oldValue, exists := os.LookupEnv(key)
	os.Setenv(key, value)
	t.Cleanup(func() {
		if exists {
			os.Setenv(key, oldValue)
		} else {
			os.Unsetenv(key)
		}
	})
	fn()
}

func TestLoadConfig_EnvSlice_ValidDestinations(t *testing.T) {
	withEnv(t, "UEBA_DESTINATIONS", "127.0.0.1:1000,example.com:2000", func() {
		cfg, err := LoadConfig("", nil)
		require.NoError(t, err)
		require.Equal(t, []string{"127.0.0.1:1000", "example.com:2000"}, cfg.Sender.Destinations)
	})
}

func TestLoadConfig_ValidateFails_InvalidEventTypes(t *testing.T) {
	withEnv(t, "UEBA_EVENT_TYPES", "login,file_access", func() {
		_, err := LoadConfig("", nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "unsupported event type")
	})
}

func TestLoadConfig_ValidateFails_NegativeRate(t *testing.T) {
	flags := &Flags{Rate: -5}
	_, err := LoadConfig("", flags)
	require.Error(t, err)
	require.Contains(t, err.Error(), "events_per_second must be positive")
}

func TestLoadConfig_ValidateFails_BufferSizeZero(t *testing.T) {
	withEnv(t, "UEBA_BUFFER_SIZE", "0", func() {
		_, err := LoadConfig("", nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "buffer_size must be positive")
	})
}

func TestLoadConfig_EnvInvalidBool(t *testing.T) {
	withEnv(t, "UEBA_PACKET_MODE", "not-bool", func() {
		_, err := LoadConfig("", nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid boolean format")
	})
}

func TestLoadConfig_EnvInvalidInt(t *testing.T) {
	withEnv(t, "UEBA_EVENTS_PER_SEC", "not-a-number", func() {
		_, err := LoadConfig("", nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid integer format")
	})
}

func TestLoadConfig_EnvInvalidDuration(t *testing.T) {
	withEnv(t, "UEBA_DURATION", "forever", func() {
		_, err := LoadConfig("", nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid duration format")
	})
}

func TestLoadConfig_EnvUnsupportedFieldType(t *testing.T) {
	loader := NewLoader()
	var dummy map[string]string
	field := reflect.ValueOf(&dummy).Elem()
	err := loader.setFieldFromEnv(field, "SOME_VAR_THAT_EXISTS")
	require.NoError(t, err)

	withEnv(t, "TEST_MAP_VAR", "value", func() {
		err := loader.setFieldFromEnv(field, "TEST_MAP_VAR")
		require.Error(t, err)
		require.Contains(t, err.Error(), "unsupported field type")
	})
}

func TestLoadConfig_ValidateFails_RateTooHigh(t *testing.T) {
	withEnv(t, "UEBA_EVENTS_PER_SEC", "300000", func() {
		_, err := LoadConfig("", nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "events_per_second too high")
	})
}

func TestGeneratorConfig_Validate_EmptyEventTypes(t *testing.T) {
	cfg := GeneratorConfig{
		EventsPerSecond: 100,
		EventTypes:      []string{}, // ← пусто!
		Duration:        time.Second,
	}
	err := cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "at least one event type")
}

func TestSenderConfig_Validate_EmptyDestinations(t *testing.T) {
	cfg := SenderConfig{
		Protocol:     "udp",
		Destinations: []string{}, // ← пусто!
		Timeout:      time.Second,
	}
	err := cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "at least one destination")
}

func TestPipelineConfig_Validate_ZeroBufferSize(t *testing.T) {
	cfg := PipelineConfig{BufferSize: 0}
	err := cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "buffer_size must be positive")
}

func TestLoadConfig_ValidateFails_InvalidTimeout(t *testing.T) {
	withEnv(t, "UEBA_TIMEOUT", "0s", func() {
		_, err := LoadConfig("", nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "timeout must be positive")
	})
}

func TestLoadConfig_ValidateFails_InvalidBufferSize(t *testing.T) {
	withEnv(t, "UEBA_BUFFER_SIZE", "-10", func() {
		_, err := LoadConfig("", nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "buffer_size must be positive")
	})
}

func TestConfigService_Load_Success(t *testing.T) {
	withEnv(t, "UEBA_PROTOCOL", "udp", func() {
		flags := &Flags{Rate: 500}
		service := NewService("testdata/valid.yaml", flags)

		err := service.Load()
		require.NoError(t, err)

		cfg := service.GetConfig()
		require.NotNil(t, cfg)
		require.Equal(t, 500, cfg.Generator.EventsPerSecond)                 // из флагов
		require.Equal(t, "udp", cfg.Sender.Protocol)                         // из env
		require.Equal(t, []string{"127.0.0.1:514"}, cfg.Sender.Destinations) // из файла
	})
}

func TestConfigService_Load_FileNotFound(t *testing.T) {
	service := NewService("nonexistent.yaml", nil)
	err := service.Load()
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to load config from file")
	require.Contains(t, err.Error(), "not found")
}

func TestConfigService_Load_ValidationFails(t *testing.T) {
	withEnv(t, "UEBA_PROTOCOL", "http", func() { // недопустимый протокол
		service := NewService("", nil)
		err := service.Load()
		require.Error(t, err)
		require.Contains(t, err.Error(), "config validate failed")
	})
}

func TestConfigService_GetConfig_BeforeLoad(t *testing.T) {
	service := NewService("", nil)
	// До Load() config — это копия DefaultConfig()
	cfg := service.GetConfig()
	require.NotNil(t, cfg)
	require.Equal(t, DefaultConfig(), cfg)
}

func TestConfigService_GetConfig_AfterLoad(t *testing.T) {
	service := NewService("testdata/valid.yaml", &Flags{Rate: 999})
	err := service.Load()
	require.NoError(t, err)

	cfg := service.GetConfig()
	require.Equal(t, 999, cfg.Generator.EventsPerSecond)
	require.Equal(t, "tcp", cfg.Sender.Protocol)
}

func TestConfigService_EmptyFile_UsesDefaultsAndEnv(t *testing.T) {
	withEnv(t, "UEBA_EVENTS_PER_SEC", "777", func() {
		service := NewService("testdata/empty.yaml", nil)
		err := service.Load()
		require.NoError(t, err)
		cfg := service.GetConfig()
		require.Equal(t, 777, cfg.Generator.EventsPerSecond)
	})
}

func TestLoadConfig_Defaults(t *testing.T) {
	cfg, err := LoadConfig("", nil)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	def := DefaultConfig()
	require.Equal(t, def, cfg)
}

func TestLoadConfig_FromFile(t *testing.T) {
	cfg, err := LoadConfig("testdata/valid.yaml", nil)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	require.Equal(t, 1000, cfg.Generator.EventsPerSecond)
	require.Equal(t, 5*time.Second, cfg.Generator.Duration)
	require.Equal(t, []string{"127.0.0.1:514"}, cfg.Sender.Destinations)
	require.Equal(t, "tcp", cfg.Sender.Protocol)
	require.Equal(t, "info", cfg.Logging.Level)
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("nonexistent.yaml", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "config file not found")
}

func TestLoadConfig_FromEnv_Overrides(t *testing.T) {
	withEnv(t, "UEBA_EVENTS_PER_SEC", "2000", func() {
		withEnv(t, "UEBA_PROTOCOL", "udp", func() {
			cfg, err := LoadConfig("", nil)
			require.NoError(t, err)
			require.Equal(t, 2000, cfg.Generator.EventsPerSecond)
			require.Equal(t, "udp", cfg.Sender.Protocol)
		})
	})
}

func TestLoadConfig_EnvTimeDuration(t *testing.T) {
	withEnv(t, "UEBA_DURATION", "10s", func() {
		cfg, err := LoadConfig("", nil)
		require.NoError(t, err)
		require.Equal(t, 10*time.Second, cfg.Generator.Duration)
	})
}

func TestLoadConfig_EnvSlice(t *testing.T) {
	withEnv(t, "UEBA_DESTINATIONS", "127.0.0.1:1000,127.0.0.1:2000,example.com:3000", func() {
		cfg, err := LoadConfig("", nil)
		require.NoError(t, err)
		require.Equal(t, []string{"127.0.0.1:1000", "127.0.0.1:2000", "example.com:3000"}, cfg.Sender.Destinations)
	})
}

func TestLoadConfig_Priority_FlagsEnvFile(t *testing.T) {
	// Файл задаёт protocol: tcp
	// Env пытается задать udp
	// Флаг должен переопределить на tcp
	withEnv(t, "UEBA_PROTOCOL", "udp", func() {
		flags := &Flags{Protocol: "tcp"}
		cfg, err := LoadConfig("testdata/valid.yaml", flags)
		require.NoError(t, err)
		require.Equal(t, "tcp", cfg.Sender.Protocol)
	})
}

func TestLoadConfig_ValidateFails_InvalidProtocol(t *testing.T) {
	withEnv(t, "UEBA_PROTOCOL", "http", func() {
		_, err := LoadConfig("", nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "validation failed")
	})
}

func TestLoadConfig_EmptyFile(t *testing.T) {
	cfg, err := LoadConfig("testdata/empty.yaml", nil)
	require.NoError(t, err)
	require.Equal(t, DefaultConfig(), cfg)
}

func TestSetSliceFromEnv_EmptyParts(t *testing.T) {
	loader := NewLoader()
	field := reflect.ValueOf(&[]string{}).Elem()
	err := loader.setSliceFromEnv(field, "  , ,  ")
	require.NoError(t, err)
	require.Equal(t, 0, field.Len())
}
