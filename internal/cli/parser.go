// Package cli отвечает за парсинг флагов командной строки.
package cli

import (
	"flag"
	"fmt"
	"time"
)

// Flags содержит значения всех поддерживаемых флагов.
type Flags struct {
	ShowVersion  bool
	ShowHelp     bool
	ConfigFile   string
	Rate         int
	Destinations string
	Protocol     string
	LogLevel     string
	Duration     time.Duration
	EventsType   string
}

// Parser инкапсулирует логику парсинга CLI-аргументов.
type Parser struct {
	version   string
	buildTime string
	flagSet   *flag.FlagSet
	flags     *Flags
}

// NewParser создаёт новый парсер с заданной версией и временем сборки.
func NewParser(version, buildTime string) *Parser {
	fs := flag.NewFlagSet("event-generator", flag.ContinueOnError)
	flags := &Flags{}

	fs.BoolVar(&flags.ShowVersion, "version", false, "Show version information")
	fs.BoolVar(&flags.ShowVersion, "v", false, "Show version (shorthand)")

	fs.BoolVar(&flags.ShowHelp, "help", false, "Show help information")
	fs.BoolVar(&flags.ShowHelp, "h", false, "Show help (shorthand)")

	fs.StringVar(&flags.ConfigFile, "config", "", "Path to configuration file")
	fs.StringVar(&flags.ConfigFile, "c", "", "Config file (shorthand)")

	fs.IntVar(&flags.Rate, "rate", 0, "Events per second (overrides config)")
	fs.IntVar(&flags.Rate, "r", 0, "Rate (shorthand)")

	fs.StringVar(&flags.Destinations, "destinations", "", "Destinations: host:port,host:port")
	fs.StringVar(&flags.Destinations, "d", "", "Destinations (shorthand)")

	fs.StringVar(&flags.Protocol, "protocol", "", "Protocol: tcp or udp")
	fs.StringVar(&flags.Protocol, "p", "", "Protocol (shorthand)")

	fs.StringVar(&flags.EventsType, "events", "", "EventsType: netflow or syslog")
	fs.StringVar(&flags.EventsType, "e", "", "EventsType (shorthand)")

	fs.StringVar(&flags.LogLevel, "log-level", "", "Log level: debug, info, warn, error")
	fs.StringVar(&flags.LogLevel, "l", "", "Log level (shorthand)")

	fs.DurationVar(&flags.Duration, "duration", 0, "How long to run (e.g., 30s, 5m, 1h)")
	fs.DurationVar(&flags.Duration, "t", 0, "Duration (shorthand)")

	return &Parser{
		version:   version,
		buildTime: buildTime,
		flagSet:   fs,
		flags:     flags,
	}
}

// Parse парсит переданный срез аргументов (обычно os.Args[1:]).
// Возвращает структуру с флагами или ошибку.
func (p *Parser) Parse(args []string) (*Flags, error) {
	if err := p.flagSet.Parse(args); err != nil {
		return nil, err
	}
	// Возвращаем копию, чтобы избежать побочных эффектов
	result := *p.flags
	return &result, nil
}

// ShowVersionInfo выводит информацию о версии.
func (p *Parser) ShowVersionInfo() {
	fmt.Printf("UEBA Event Generator\n")
	fmt.Printf("Version: %s\n", p.version)
	fmt.Printf("Build Time: %s\n", p.buildTime)
}

// ShowHelpInfo выводит справку по использованию.
func (p *Parser) ShowHelpInfo() {
	fmt.Printf("UEBA Event Generator - Generate synthetic security events\n\n")
	fmt.Printf("USAGE:\n")
	fmt.Printf("  ueba-generator [FLAGS]\n\n")
	fmt.Printf("FLAGS:\n")
	fmt.Printf("  -c, --config FILE         Configuration file path\n")
	fmt.Printf("  -r, --rate N              Events per second\n")
	fmt.Printf("  -d, --destinations LIST   Comma-separated destinations\n")
	fmt.Printf("  -p, --protocol PROTO      Protocol: tcp or udp\n")
	fmt.Printf("  -e, --events TYPE         Events type: netflow or syslog\n")
	fmt.Printf("  -t, --duration TIME       How long to run (e.g., 30s, 5m, 1h)\n")
	fmt.Printf("  -l, --log-level LEVEL     Log level: debug, info, warn, error\n")
	fmt.Printf("  -v, --version             Show version information\n")
	fmt.Printf("  -h, --help                Show this help\n\n")
	fmt.Printf("EXAMPLES:\n")
	fmt.Printf("  ueba-generator -c config.yaml\n")
	fmt.Printf("  ueba-generator -r 100 -d 127.0.0.1:514\n")
	fmt.Printf("  ueba-generator -r 50 -p tcp -d 10.0.1.100:514\n")
}
