// Package cli implements the command-line inteface
package cli

import (
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/nashabanov/ueba-event-generator/cmd/app"
	"github.com/nashabanov/ueba-event-generator/internal/config"
	"github.com/nashabanov/ueba-event-generator/internal/logger"
	"github.com/nashabanov/ueba-event-generator/internal/monitoring"
	"github.com/nashabanov/ueba-event-generator/internal/pipeline/factory"
)

// Версионная информация (заполняется при сборке)
var (
	version   = "dev"     // Версия приложения
	buildTime = "unknown" // Время сборки
	gitCommit = "unknown" // Git коммит
)

type CLIFlags struct {
	// Системные флаги
	showVersion bool
	showHelp    bool

	// Конфигурационные флаги
	configFile   string
	rate         int
	destinations string
	protocol     string
	logLevel     string
	duration     time.Duration
}

func Run() {
	flags := parseComandLineFlags()

	// Обрабатываем специальные флаги
	if flags.showVersion {
		showVersionInfo()
		return
	}
	if flags.showHelp {
		showHelpInfo()
		return
	}

	log := logger.NewStdLogger()

	// Загружаем конфигурацию
	cfg, err := loadConfiguration(flags)
	if err != nil {
		log.Error("Failed to load configuration: %v", err)
	}

	printConfigInfo(cfg, flags)

	// Создаем pipeline
	factory := factory.NewPipelineFactory(cfg)
	pipeline, err := factory.CreatePipeline()
	if err != nil {
		log.Error("Failed to build pipeline: %v", err)
	}

	// Создаем monitoring
	monitoring := monitoring.NewMonitor(10*time.Second, log)

	// Создаем и запускаем приложение
	app := app.NewApplication(cfg, pipeline, monitoring, log)

	if err := app.Run(); err != nil {
		log.Error("Application failed: %v", err)
	}

	fmt.Println("Configuration loaded successfully!")
	fmt.Println("Next step: create pipeline...")
}

// parseCommandLineFlags парсит все флаги
func parseComandLineFlags() *CLIFlags {
	flags := &CLIFlags{}

	// Системные флаги
	flag.BoolVar(&flags.showVersion, "version", false, "Show version information")
	flag.BoolVar(&flags.showVersion, "v", false, "Show version (shorthand)")

	flag.BoolVar(&flags.showHelp, "help", false, "Show help information")
	flag.BoolVar(&flags.showHelp, "h", false, "Show help (shorthand)")

	flag.StringVar(&flags.configFile, "config", "", "Path to configuration file")
	flag.StringVar(&flags.configFile, "c", "", "Config file (shorthand)")

	flag.IntVar(&flags.rate, "rate", 0, "Events per second (overrides config)")
	flag.IntVar(&flags.rate, "r", 0, "Rate (shorthand)")

	flag.StringVar(&flags.destinations, "destinations", "", "Destinations: host:port,host:port")
	flag.StringVar(&flags.destinations, "d", "", "Destinations (shorthand)")

	flag.StringVar(&flags.protocol, "protocol", "", "Protocol: tcp or udp")
	flag.StringVar(&flags.protocol, "p", "", "Protocol (shorthand)")

	flag.StringVar(&flags.logLevel, "log-level", "", "Log level: debug, info, warn, error")
	flag.StringVar(&flags.logLevel, "l", "", "Log level (shorthand)")

	flag.DurationVar(&flags.duration, "duration", 0, "How long to run (e.g., 30s, 5m, 1h)")
	flag.DurationVar(&flags.duration, "t", 0, "Duration (shorthand)")

	flag.Parse()
	return flags
}

// loadConfiguration загружает конфигурацию из всех источников
func loadConfiguration(flags *CLIFlags) (*config.Config, error) {
	configFlags := &config.Flags{
		ConfigFile: flags.configFile,
		Rate:       flags.rate,
		Protocol:   flags.protocol,
		LogLevel:   flags.logLevel,
		Duration:   flags.duration,
	}

	if flags.destinations != "" {
		parts := strings.Split(flags.destinations, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		configFlags.Destinations = parts
	}

	service := config.NewConfigService(flags.configFile, configFlags)

	if err := service.Load(); err != nil {
		return nil, err
	}

	return service.GetConfig(), nil
}

// printConfigInfo выводит информацию о загруженной конфигурации
func printConfigInfo(cfg *config.Config, flags *CLIFlags) {
	fmt.Printf("=== Configuration Loaded ===\n")

	// Показываем источник конфигурации
	if flags.configFile != "" {
		fmt.Printf("Config source: %s + environment + CLI flags\n", flags.configFile)
	} else {
		fmt.Printf("Config source: defaults + environment + CLI flags\n")
	}

	// Основные параметры
	fmt.Printf("Generator: %d events/sec\n", cfg.Generator.EventsPerSecond)
	fmt.Printf("Event types: %v\n", cfg.Generator.EventTypes)

	if cfg.Generator.Duration > 0 {
		fmt.Printf("Duration: %v\n", cfg.Generator.Duration)
	} else {
		fmt.Printf("Duration: unlimited\n")
	}

	fmt.Printf("Sender: %s protocol\n", cfg.Sender.Protocol)
	fmt.Printf("Destinations: %v\n", cfg.Sender.Destinations)
	fmt.Printf("Pipeline: buffer=%d", cfg.Pipeline.BufferSize)
	fmt.Printf("Logging: level=%s, format=%s\n", cfg.Logging.Level, cfg.Logging.Format)

	fmt.Printf("===============================\n\n")
}

// showVersionInfo выводит информацию о версии
func showVersionInfo() {
	fmt.Printf("UEBA Event Generator\n")
	fmt.Printf("Version: %s\n", version)
	fmt.Printf("Build Time: %s\n", buildTime)
	fmt.Printf("Git Commit: %s\n", gitCommit)
}

// showHelpInfo выводит базовую справку
func showHelpInfo() {
	fmt.Printf("UEBA Event Generator - Generate synthetic security events\n\n")
	fmt.Printf("USAGE:\n")
	fmt.Printf("  ueba-generator [FLAGS]\n\n")
	fmt.Printf("FLAGS:\n")
	fmt.Printf("  -c, --config FILE         Configuration file path\n")
	fmt.Printf("  -r, --rate N             Events per second\n")
	fmt.Printf("  -d, --destinations LIST  Comma-separated destinations\n")
	fmt.Printf("  -p, --protocol PROTO     Protocol: tcp or udp\n")
	fmt.Printf("  -t, --duration TIME      How long to run (e.g., 30s, 5m, 1h)\n")
	fmt.Printf("  -l, --log-level LEVEL    Log level: debug, info, warn, error\n")
	fmt.Printf("  -v, --version            Show version information\n")
	fmt.Printf("  -h, --help               Show this help\n\n")
	fmt.Printf("EXAMPLES:\n")
	fmt.Printf("  ueba-generator -c config.yaml\n")
	fmt.Printf("  ueba-generator -r 100 -d 127.0.0.1:514\n")
	fmt.Printf("  ueba-generator -r 50 -p tcp -d 10.0.1.100:514\n")
}
