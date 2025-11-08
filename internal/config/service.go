package config

import (
	"fmt"
)

// Service для работы с конфигурацией
type Service interface {
	Load() error
	GetConfig() *Config
}

// configService реализация Service
type configService struct {
	config   *Config
	loader   *Loader
	filePath string
	flags    *Flags
}

// NewService создает новый экземпляр Service
func NewService(filePath string, flags *Flags) Service {
	return &configService{
		config:   DefaultConfig(),
		loader:   NewLoader(),
		filePath: filePath,
		flags:    flags,
	}
}

// Load загружает конфигурацию из файла, окружения, флагов и валидирует её
func (cs *configService) Load() error {
	if err := cs.loader.LoadFromFile(cs.filePath); err != nil {
		return fmt.Errorf("failed to load config from file %s: %w", cs.filePath, err)
	}

	if err := cs.loader.LoadFromEnv(); err != nil {
		return fmt.Errorf("failed to load config from env: %w", err)
	}

	cs.loader.ApplyFlags(cs.flags)

	cs.config = cs.loader.GetConfig()

	if err := cs.config.Validate(); err != nil {
		return fmt.Errorf("config validate failed: %w", err)
	}

	return nil
}

// GetConfig возвращает текущую конфигурацию
func (cs *configService) GetConfig() *Config {
	return cs.config
}
