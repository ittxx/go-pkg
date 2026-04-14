package config

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// Source represents a configuration source
type Source interface {
	Load() (map[string]interface{}, error)
}

// ConfigManager manages configuration from multiple sources
type ConfigManager struct {
	sources []Source
}

// New creates a new configuration manager
func New() *ConfigManager {
	return &ConfigManager{
		sources: make([]Source, 0),
	}
}

// AddSource adds a configuration source
func (cm *ConfigManager) AddSource(source Source) *ConfigManager {
	cm.sources = append(cm.sources, source)
	return cm
}

// Load loads configuration into the target struct
func (cm *ConfigManager) Load(config interface{}) error {
	// 1. Set default values
	if err := setDefaults(config); err != nil {
		return fmt.Errorf("failed to set defaults: %w", err)
	}

	// 2. Load from sources in priority order (later sources override earlier ones)
	for _, source := range cm.sources {
		data, err := source.Load()
		if err != nil {
			return fmt.Errorf("failed to load from source: %w", err)
		}

		if err := mergeConfig(config, data); err != nil {
			return fmt.Errorf("failed to merge config: %w", err)
		}
	}

	// 3. Validate configuration
	if err := validateConfig(config); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	return nil
}

// LoadFromSources loads configuration from multiple sources with priority
func LoadFromSources(target interface{}, sources ...Source) error {
	return New().AddSource(sources...).Load(target)
}

// mergeConfig merges source data into target struct
func mergeConfig(target interface{}, source map[string]interface{}) error {
	v := reflect.ValueOf(target)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return errors.New("target must be a struct or pointer to struct")
	}

	return mergeStruct(v, source, "")
}

func mergeStruct(v reflect.Value, source map[string]interface{}, prefix string) error {
	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := t.Field(i)

		// Skip unexported fields
		if !field.CanSet() {
			continue
		}

		// Handle nested structs
		if field.Kind() == reflect.Struct {
			// Special handling for time.Duration
			if field.Type() == reflect.TypeOf(time.Duration(0)) {
				// Build the struct name prefix for flattened keys
				fieldPrefix := ""
				if prefix != "" {
					fieldPrefix = prefix + strings.Title(fieldType.Name)
				} else {
					fieldPrefix = strings.Title(fieldType.Name)
				}

				// Try to find value in source
				var possibleKeys []string
				possibleKeys = append(possibleKeys, fieldPrefix)
				
				// Try yaml tag
				if yamlTag := fieldType.Tag.Get("yaml"); yamlTag != "" && yamlTag != "-" {
					if prefix != "" {
						possibleKeys = append(possibleKeys, prefix+strings.Title(yamlTag))
					} else {
						possibleKeys = append(possibleKeys, strings.Title(yamlTag))
					}
				}

				// Look for value in source
				var foundValue interface{}
				var found bool
				for _, key := range possibleKeys {
					if value, exists := source[key]; exists {
						foundValue = value
						found = true
						break
					}
				}

				if found {
					if err := setDurationValue(field, foundValue); err != nil {
						return fmt.Errorf("failed to set duration field %s: %w", fieldType.Name, err)
					}
				}
				continue
			}

			// Build the struct name prefix for flattened keys
			structPrefix := ""
			if prefix != "" {
				structPrefix = prefix + strings.Title(fieldType.Name)
			} else {
				structPrefix = strings.Title(fieldType.Name)
			}

			// Try to merge nested struct
			if err := mergeStruct(field, source, structPrefix); err != nil {
				return fmt.Errorf("%s.%s", fieldType.Name, err.Error())
			}
			continue
		}

		// Handle regular fields
		yamlTag := fieldType.Tag.Get("yaml")
		envTag := fieldType.Tag.Get("env")

		// Try multiple key strategies
		var possibleKeys []string

		// 1. Try flattened key from YAML (e.g., "AppServerHost")
		if prefix != "" {
			possibleKeys = append(possibleKeys, prefix+strings.Title(fieldType.Name))
		} else {
			possibleKeys = append(possibleKeys, strings.Title(fieldType.Name))
		}

		// 2. Try yaml tag
		if yamlTag != "" && yamlTag != "-" {
			if prefix != "" {
				possibleKeys = append(possibleKeys, prefix+strings.Title(yamlTag))
			} else {
				possibleKeys = append(possibleKeys, strings.Title(yamlTag))
			}
		}

		// 3. Try env tag with prefix
		if envTag != "" {
			if prefix != "" {
				possibleKeys = append(possibleKeys, prefix+strings.Title(envTag))
			} else {
				possibleKeys = append(possibleKeys, strings.Title(envTag))
			}
		}

		// Try keys without the top-level prefix for ENV compatibility
		if prefix != "" && strings.Contains(prefix, "App") {
			// Extract the struct name prefix (remove "App" from "AppDatabase")
			structOnlyPrefix := strings.TrimPrefix(prefix, "App")
			// Try with struct prefix only (e.g., "DatabaseHost")
			possibleKeys = append(possibleKeys, structOnlyPrefix+strings.Title(fieldType.Name))

			// Try with yaml tag without "App" prefix
			if yamlTag != "" && yamlTag != "-" {
				possibleKeys = append(possibleKeys, structOnlyPrefix+strings.Title(yamlTag))
			}
		}

		// Look for value in source using any of the possible keys
		var foundValue interface{}
		var found bool
		for _, key := range possibleKeys {
			if value, exists := source[key]; exists {
				foundValue = value
				found = true
				break
			}
		}

		// Always set the value if found (env overrides yaml)
		if found {
			if err := setFieldValue(field, foundValue); err != nil {
				return fmt.Errorf("failed to set field %s: %w", fieldType.Name, err)
			}
		}
	}

	return nil
}

// validateConfig validates the configuration struct
func validateConfig(config interface{}) error {
	v := reflect.ValueOf(config)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return errors.New("config must be a struct or pointer to struct")
	}

	return validateStruct(v)
}

func validateStruct(v reflect.Value) error {
	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := t.Field(i)

		// Check required tag
		if required := fieldType.Tag.Get("required"); required == "true" {
			if isZero(field) {
				return fmt.Errorf("field %s is required", fieldType.Name)
			}
		}

		// Recursive validation for nested structs
		if field.Kind() == reflect.Struct {
			if field.Type() != reflect.TypeOf(time.Duration(0)) { // Skip time.Duration
				if err := validateStruct(field); err != nil {
					return fmt.Errorf("%s.%s", fieldType.Name, err.Error())
				}
			}
		}
	}

	return nil
}

// setDefaults sets default values from struct tags
func setDefaults(config interface{}) error {
	v := reflect.ValueOf(config)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return errors.New("config must be a struct or pointer to struct")
	}

	return setStructDefaults(v)
}

func setStructDefaults(v reflect.Value) error {
	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := t.Field(i)

		// Check default tag
		if defaultValue := fieldType.Tag.Get("default"); defaultValue != "" {
			if isZero(field) {
				if field.Type() == reflect.TypeOf(time.Duration(0)) {
					if err := setDurationValue(field, defaultValue); err != nil {
						return fmt.Errorf("failed to set default for %s: %w", fieldType.Name, err)
					}
				} else {
					if err := setFieldValue(field, defaultValue); err != nil {
						return fmt.Errorf("failed to set default for %s: %w", fieldType.Name, err)
					}
				}
			}
		}

		// Recursive defaults for nested structs
		if field.Kind() == reflect.Struct && field.Type() != reflect.TypeOf(time.Duration(0)) {
			if err := setStructDefaults(field); err != nil {
				return err
			}
		}
	}

	return nil
}

func setFieldValue(field reflect.Value, value interface{}) error {
	if !field.CanSet() {
		return fmt.Errorf("field is not settable")
	}

	var strValue string
	switch v := value.(type) {
	case string:
		strValue = v
	case float64:
		// JSON numbers are float64 by default
		if field.Kind() == reflect.Int || field.Kind() == reflect.Int64 {
			field.SetInt(int64(v))
			return nil
		}
		strValue = fmt.Sprintf("%v", v)
	case bool:
		if field.Kind() == reflect.Bool {
			field.SetBool(v)
			return nil
		}
		strValue = fmt.Sprintf("%v", v)
	default:
		strValue = fmt.Sprintf("%v", value)
	}

	return setFieldValueFromString(field, strValue)
}

func setFieldValueFromString(field reflect.Value, value string) error {
	switch field.Kind() {
	case reflect.String:
		field.SetString(value)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if intValue, err := strconv.ParseInt(value, 10, 64); err == nil {
			field.SetInt(intValue)
		} else {
			return err
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if uintValue, err := strconv.ParseUint(value, 10, 64); err == nil {
			field.SetUint(uintValue)
		} else {
			return err
		}
	case reflect.Float32, reflect.Float64:
		if floatValue, err := strconv.ParseFloat(value, 64); err == nil {
			field.SetFloat(floatValue)
		} else {
			return err
		}
	case reflect.Bool:
		if boolValue, err := strconv.ParseBool(value); err == nil {
			field.SetBool(boolValue)
		} else {
			return err
		}
	default:
		return fmt.Errorf("unsupported field type: %s", field.Kind())
	}

	return nil
}

func setDurationValue(field reflect.Value, value interface{}) error {
	if !field.CanSet() {
		return fmt.Errorf("duration field is not settable")
	}

	var strValue string
	switch v := value.(type) {
	case string:
		strValue = v
	case int, int64:
		strValue = fmt.Sprintf("%ds", v)
	case float64:
		strValue = fmt.Sprintf("%.0fs", v)
	default:
		strValue = fmt.Sprintf("%v", value)
	}

	duration, err := time.ParseDuration(strValue)
	if err != nil {
		return fmt.Errorf("failed to parse duration: %w", err)
	}

	field.SetInt(int64(duration))
	return nil
}

func isZero(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.String:
		return v.String() == ""
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Ptr, reflect.Interface:
		return v.IsNil()
	default:
		return reflect.DeepEqual(v.Interface(), reflect.Zero(v.Type()).Interface())
	}
}

// Convenience functions for common usage

// LoadFromYAML loads configuration from a single YAML file
func LoadFromYAML(configPath string, target interface{}) error {
	return New().AddSource(NewYAMLSource(configPath)).Load(target)
}

// LoadFromENV loads configuration from environment variables
func LoadFromENV(target interface{}) error {
	return New().AddSource(NewEnvSource("")).Load(target)
}

// LoadFromYAMLAndENV loads configuration from YAML file and environment variables
// Environment variables will override YAML values
func LoadFromYAMLAndENV(configPath string, target interface{}) error {
	return New().
		AddSource(NewYAMLSource(configPath)).
		AddSource(NewEnvSource("")).
		Load(target)
}
