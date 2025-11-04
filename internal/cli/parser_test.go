package cli

import (
	"bytes"
	"testing"
	"time"
)

func TestParser_Parse(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected *Flags
		wantErr  bool
	}{
		{
			name: "valid long flags",
			args: []string{
				"--config", "test.yaml",
				"--rate", "100",
				"--destinations", "127.0.0.1:514",
				"--protocol", "tcp",
				"--log-level", "debug",
				"--duration", "30s",
			},
			expected: &Flags{
				ConfigFile:   "test.yaml",
				Rate:         100,
				Destinations: "127.0.0.1:514",
				Protocol:     "tcp",
				LogLevel:     "debug",
				Duration:     30 * time.Second,
			},
		},
		{
			name: "valid short flags",
			args: []string{
				"-c", "short.yaml",
				"-r", "200",
				"-d", "10.0.0.1:9999",
				"-p", "udp",
				"-l", "warn",
				"-t", "5m",
			},
			expected: &Flags{
				ConfigFile:   "short.yaml",
				Rate:         200,
				Destinations: "10.0.0.1:9999",
				Protocol:     "udp",
				LogLevel:     "warn",
				Duration:     5 * time.Minute,
			},
		},
		{
			name:     "show version",
			args:     []string{"--version"},
			expected: &Flags{ShowVersion: true},
		},
		{
			name:     "show help",
			args:     []string{"-h"},
			expected: &Flags{ShowHelp: true},
		},
		{
			name:    "invalid flag",
			args:    []string{"--unknown-flag"},
			wantErr: true,
		},
		{
			name:     "empty args",
			args:     []string{},
			expected: &Flags{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser("test-version", "test-build-time")
			parser.flagSet.SetOutput(new(bytes.Buffer))

			flags, err := parser.Parse(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if *flags != *tt.expected {
				t.Errorf("got %+v, want %+v", flags, tt.expected)
			}
		})
	}
}

// Smoke-тесты для функций вывода (они не должны паниковать)
func TestParser_ShowVersionInfo(t *testing.T) {
	parser := NewParser("1.0.0", "2025-01-01T00:00:00Z")
	parser.ShowVersionInfo()
}

func TestParser_ShowHelpInfo(t *testing.T) {
	parser := NewParser("1.0.0", "2025-01-01T00:00:00Z")
	parser.ShowHelpInfo()
}
