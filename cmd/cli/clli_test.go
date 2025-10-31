package cli

import (
	"bytes"
	"flag"
	"os"
	"testing"
	"time"

	"github.com/nashabanov/ueba-event-generator/internal/config"
	"github.com/stretchr/testify/require"
)

// resetFlags сбрасывает глобальный парсер
func resetFlags() {
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
}

func captureOutput(f func()) string {
	var buf bytes.Buffer
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	w.Close()
	os.Stdout = old
	buf.ReadFrom(r)
	return buf.String()
}

func TestParseCommandLineFlags(t *testing.T) {
	resetFlags()
	os.Args = []string{"cmd", "-r", "100", "-d", "127.0.0.1:514", "-p", "tcp", "-t", "30s"}

	flags := parseComandLineFlags()

	require.Equal(t, 100, flags.rate)
	require.Equal(t, "127.0.0.1:514", flags.destinations)
	require.Equal(t, "tcp", flags.protocol)
	require.Equal(t, 30*time.Second, flags.duration)
}

func TestLoadConfiguration_WithDestinations(t *testing.T) {
	flags := &CLIFlags{
		rate:         42,
		destinations: "127.0.0.1:514, 10.0.0.1:9999",
		protocol:     "udp",
		logLevel:     "debug",
	}

	cfg, err := loadConfiguration(flags)
	require.NoError(t, err)
	require.IsType(t, &config.Config{}, cfg)
	require.Equal(t, []string{"127.0.0.1:514", "10.0.0.1:9999"}, cfg.Sender.Destinations)
	require.Equal(t, "udp", cfg.Sender.Protocol)
}

func TestPrintConfigInfo(t *testing.T) {
	cfg := &config.Config{
		Generator: config.GeneratorConfig{
			EventsPerSecond: 50,
			EventTypes:      []string{"login", "logout"},
			Duration:        10 * time.Second,
		},
		Sender: config.SenderConfig{
			Protocol:     "tcp",
			Destinations: []string{"localhost:514"},
		},
		Pipeline: config.PipelineConfig{
			BufferSize: 10,
		},
		Logging: config.LoggingConfig{
			Level:  "info",
			Format: "text",
		},
	}

	flags := &CLIFlags{
		configFile: "test.yaml",
	}

	out := captureOutput(func() {
		printConfigInfo(cfg, flags)
	})

	require.Contains(t, out, "Config source: test.yaml")
	require.Contains(t, out, "Generator: test-gen (50 events/sec)")
	require.Contains(t, out, "Duration: 10s")
	require.Contains(t, out, "Sender: test-sender (tcp protocol)")
	require.Contains(t, out, "Logging: level=info")
}

func TestShowVersionInfo(t *testing.T) {
	out := captureOutput(showVersionInfo)
	require.Contains(t, out, "UEBA Event Generator")
	require.Contains(t, out, "Version:")
}

func TestShowHelpInfo(t *testing.T) {
	out := captureOutput(showHelpInfo)
	require.Contains(t, out, "USAGE:")
	require.Contains(t, out, "FLAGS:")
	require.Contains(t, out, "EXAMPLES:")
}

func TestRun_WithVersionFlag(t *testing.T) {
	resetFlags()
	os.Args = []string{"cmd", "--version"}

	out := captureOutput(Run)

	require.Contains(t, out, "Version:")
}

func TestRun_WithHelpFlag(t *testing.T) {
	resetFlags()
	os.Args = []string{"cmd", "--help"}

	out := captureOutput(Run)

	require.Contains(t, out, "USAGE:")
	require.Contains(t, out, "FLAGS:")
}
