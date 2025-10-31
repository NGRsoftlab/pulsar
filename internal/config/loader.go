package config

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Loader загружает конфигурацию из разных источников
type Loader struct {
	config *Config
}

// NewLoader создает новый загрузчик с дефолтными значениями
func NewLoader() *Loader {
	return &Loader{
		config: DefaultConfig(),
	}
}

// LoadFromFile загружает конфигурацию из YAML файла
func (l *Loader) LoadFromFile(filename string) error {
	if filename == "" {
		return nil // использем defaults
	}

	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return fmt.Errorf("config file not found: %s", filename)
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read config file: %s", filename)
	}

	if err := yaml.Unmarshal(data, l.config); err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	return nil
}

// LoadFromEnv загружает конфигурацию из переменных окружения
func (l *Loader) LoadFromEnv() error {
	return l.loadEnvVars(l.config)
}

// Flags структура для command-line флагов
type Flags struct {
	ConfigFile   string
	Rate         int
	Destinations []string
	Protocol     string
	BufferSize   int
	LogLevel     string
	Duration     time.Duration
}

// ApplyFlags применяет флаги командной строки (переопределяют все остальное)
func (l *Loader) ApplyFlags(flags *Flags) {
	if flags.Rate > 0 {
		l.config.Generator.EventsPerSecond = flags.Rate
	}

	if len(flags.Destinations) > 0 {
		l.config.Sender.Destinations = flags.Destinations
	}

	if flags.Protocol != "" {
		l.config.Sender.Protocol = flags.Protocol
	}

	if flags.BufferSize > 0 {
		l.config.Pipeline.BufferSize = flags.BufferSize
	}

	if flags.LogLevel != "" {
		l.config.Logging.Level = flags.LogLevel
	}

	if flags.Duration > 0 {
		l.config.Generator.Duration = flags.Duration
	}
}

// GetConfig возвращает итоговую конфигурацию
func (l *Loader) GetConfig() *Config {
	return l.config
}

// loadEnvVars загружает переменные окружения используя рефлексию
func (l *Loader) loadEnvVars(config any) error {
	return l.loadEnvVarsRecursive(reflect.ValueOf(config), "")
}

// loadEnvVarsRecursive рекурсивно обрабатывает структуры
func (l *Loader) loadEnvVarsRecursive(v reflect.Value, prefix string) error {
	// Разыменовываем указатели
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return nil
	}

	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := t.Field(i)

		// Пропускаем неэкспортируемые поля
		if !fieldType.IsExported() {
			continue
		}

		// Проверяем наличие env тега
		envTag := fieldType.Tag.Get("env")

		if envTag != "" {
			// Загружаем значение из переменной окружения
			if err := l.setFieldFromEnv(field, envTag); err != nil {
				return fmt.Errorf("failed to set field %s from env %s: %w",
					fieldType.Name, envTag, err)
			}
		} else if field.Kind() == reflect.Struct ||
			(field.Kind() == reflect.Ptr && field.Type().Elem().Kind() == reflect.Struct) {
			// Рекурсивно обрабатываем вложенные структуры
			if err := l.loadEnvVarsRecursive(field, prefix+fieldType.Name+"_"); err != nil {
				return err
			}
		}
	}

	return nil
}

// setFieldFromEnv устанавливает значение поля из переменной окружения
func (l *Loader) setFieldFromEnv(field reflect.Value, envVar string) error {
	if !field.CanSet() {
		return fmt.Errorf("field cannot be set")
	}

	envValue := os.Getenv(envVar)
	if envValue == "" {
		return nil // Переменная не установлена - используем дефолт
	}

	switch field.Kind() {
	case reflect.String:
		field.SetString(envValue)

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if field.Type() == reflect.TypeOf(time.Duration(0)) {
			// Специальная обработка для time.Duration
			duration, err := time.ParseDuration(envValue)
			if err != nil {
				return fmt.Errorf("invalid duration format: %w", err)
			}
			field.SetInt(int64(duration))
		} else {
			intVal, err := strconv.ParseInt(envValue, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid integer format: %w", err)
			}
			field.SetInt(intVal)
		}

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		uintVal, err := strconv.ParseUint(envValue, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid unsigned integer format: %w", err)
		}
		field.SetUint(uintVal)

	case reflect.Bool:
		boolVal, err := strconv.ParseBool(envValue)
		if err != nil {
			return fmt.Errorf("invalid boolean format: %w", err)
		}
		field.SetBool(boolVal)

	case reflect.Float32, reflect.Float64:
		floatVal, err := strconv.ParseFloat(envValue, field.Type().Bits())
		if err != nil {
			return fmt.Errorf("invalid float format: %w", err)
		}
		field.SetFloat(floatVal)

	case reflect.Slice:
		return l.setSliceFromEnv(field, envValue)

	default:
		return fmt.Errorf("unsupported field type: %s", field.Kind())
	}

	return nil
}

// setSliceFromEnv устанавливает slice из comma-separated строки
func (l *Loader) setSliceFromEnv(field reflect.Value, envValue string) error {
	if envValue == "" {
		return nil
	}

	// Разделяем по запятым и очищаем пробелы
	parts := strings.Split(envValue, ",")
	for i, part := range parts {
		parts[i] = strings.TrimSpace(part)
	}

	// Фильтруем пустые строки
	var cleanParts []string
	for _, part := range parts {
		if part != "" {
			cleanParts = append(cleanParts, part)
		}
	}

	if len(cleanParts) == 0 {
		return nil
	}

	elemType := field.Type().Elem()
	slice := reflect.MakeSlice(field.Type(), len(cleanParts), len(cleanParts))

	for i, part := range cleanParts {
		elem := slice.Index(i)

		switch elemType.Kind() {
		case reflect.String:
			elem.SetString(part)

		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			intVal, err := strconv.ParseInt(part, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid integer in slice: %s", part)
			}
			elem.SetInt(intVal)

		default:
			return fmt.Errorf("unsupported slice element type: %s", elemType.Kind())
		}
	}

	field.Set(slice)
	return nil
}

// LoadConfig удобная функция для полной загрузки конфигурации
func LoadConfig(configFile string, flags *Flags) (*Config, error) {
	loader := NewLoader()

	// 1. Загружаем из файла (если указан)
	if err := loader.LoadFromFile(configFile); err != nil {
		return nil, fmt.Errorf("failed to load config file: %w", err)
	}

	// 2. Применяем переменные окружения
	if err := loader.LoadFromEnv(); err != nil {
		return nil, fmt.Errorf("failed to load environment variables: %w", err)
	}

	// 3. Применяем флаги командной строки (наивысший приоритет)
	if flags != nil {
		loader.ApplyFlags(flags)
	}

	// 4. Валидируем итоговую конфигурацию
	config := loader.GetConfig()
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return config, nil
}
