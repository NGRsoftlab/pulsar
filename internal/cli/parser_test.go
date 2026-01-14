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
				"--events", "netflow",
				"--log-level", "debug",
				"--duration", "30s",
			},
			expected: &Flags{
				ConfigFile:   "test.yaml",
				Rate:         100,
				Destinations: "127.0.0.1:514",
				Protocol:     "tcp",
				EventsType:   "netflow",
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
				"-e", "syslog",
				"-l", "warn",
				"-t", "5m",
			},
			expected: &Flags{
				ConfigFile:   "short.yaml",
				Rate:         200,
				Destinations: "10.0.0.1:9999",
				Protocol:     "udp",
				EventsType:   "syslog",
				LogLevel:     "warn",
				Duration:     5 * time.Minute,
			},
		},
		{
			name: "show version",
			args: []string{"--version"},
			expected: &Flags{
				ShowVersion:  true,
				Rate:         10,
				Destinations: "127.0.0.1:514",
				Protocol:     "udp",
				EventsType:   "netflow",
				LogLevel:     "info",
			},
		},
		{
			name: "show help",
			args: []string{"-h"},
			expected: &Flags{
				ShowHelp:     true,
				Rate:         10,
				Destinations: "127.0.0.1:514",
				Protocol:     "udp",
				EventsType:   "netflow",
				LogLevel:     "info",
			},
		},
		{
			name:    "invalid flag",
			args:    []string{"--unknown-flag"},
			wantErr: true,
		},
		{
			name: "empty args (use defaults)",
			args: []string{},
			expected: &Flags{
				Rate:         10,
				Destinations: "127.0.0.1:514",
				Protocol:     "udp",
				EventsType:   "netflow",
				LogLevel:     "info",
			},
		},
		{
			name: "partial flags (mix defaults and overrides)",
			args: []string{"-r", "500", "-p", "tcp"},
			expected: &Flags{
				Rate:         500,
				Destinations: "127.0.0.1:514",
				Protocol:     "tcp",
				EventsType:   "netflow",
				LogLevel:     "info",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser("test-version")
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
	parser := NewParser("1.0.0")
	parser.ShowVersionInfo()
}

func TestParser_ShowHelpInfo(t *testing.T) {
	parser := NewParser("1.0.0")
	parser.ShowHelpInfo()
}
