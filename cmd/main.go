package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nashabanov/ueba-event-generator/internal/cli"
	"github.com/nashabanov/ueba-event-generator/internal/config"
	"github.com/nashabanov/ueba-event-generator/internal/lifecycle"
	"github.com/nashabanov/ueba-event-generator/internal/logger"
	"github.com/nashabanov/ueba-event-generator/internal/metrics"
	"github.com/nashabanov/ueba-event-generator/internal/monitoring"
	"github.com/nashabanov/ueba-event-generator/internal/pipeline/factory"
)

// Версионная информация (заполняется при сборке)
var (
	version   = "dev"     // Версия приложения
	buildTime = "unknown" // Время сборки
)

func main() {
	if err := run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Парсим CLI флаги
	parser := cli.NewParser(version, buildTime)
	flags, err := parser.Parse(os.Args[1:])
	if err != nil {
		return fmt.Errorf("failde to parse CLI flags: %v", err)
	}

	if flags.ShowVersion {
		parser.ShowVersionInfo()
		return nil
	}

	if flags.ShowHelp {
		parser.ShowHelpInfo()
		return nil
	}

	// Инциализируем логгер
	log := logger.NewStdLogger()

	// Загружаем конфигурацию
	cfg, err := loadConfiguration(flags)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %v", err)
	}

	printConfigInfo(cfg, flags)

	fmt.Println("Configuration loaded successfully!")
	fmt.Println("Next step: create pipeline...")

	// Создаем pipeline
	factory := factory.NewPipelineFactory(cfg, metrics.GetGlobalMetrics())
	pipeline, err := factory.CreatePipeline()
	if err != nil {
		return fmt.Errorf("failed to build pipeline: %v", err)
	}

	// Создаем monitoring
	monitoring := monitoring.NewMonitor(10*time.Second, log)

	// Создаем и запускаем приложение
	manager := lifecycle.NewManager(pipeline, monitoring, log)

	if err := manager.Run(cfg.Generator.Duration); err != nil {
		return fmt.Errorf("application failed: %v", err)
	}

	return nil
}

// loadConfiguration загружает конфигурацию из всех источников
func loadConfiguration(flags *cli.Flags) (*config.Config, error) {
	configFlags := &config.Flags{
		ConfigFile: flags.ConfigFile,
		Rate:       flags.Rate,
		Protocol:   flags.Protocol,
		LogLevel:   flags.LogLevel,
		Duration:   flags.Duration,
	}

	if flags.Destinations != "" {
		parts := strings.Split(flags.Destinations, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		configFlags.Destinations = parts
	}

	service := config.NewConfigService(flags.ConfigFile, configFlags)

	if err := service.Load(); err != nil {
		return nil, err
	}

	return service.GetConfig(), nil
}

// printConfigInfo выводит информацию о загруженной конфигурации
func printConfigInfo(cfg *config.Config, flags *cli.Flags) {
	fmt.Printf("=== Configuration Loaded ===\n")

	// Показываем источник конфигурации
	if flags.ConfigFile != "" {
		fmt.Printf("Config source: %s + environment + CLI flags\n", flags.ConfigFile)
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
	fmt.Printf("Pipeline: buffer=%d\n", cfg.Pipeline.BufferSize)
	fmt.Printf("Logging: level=%s, format=%s\n", cfg.Logging.Level, cfg.Logging.Format)

	fmt.Printf("===============================\n\n")
}
