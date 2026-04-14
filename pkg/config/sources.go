package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// YAMLSource loads configuration from YAML file
type YAMLSource struct {
	path string
}

// NewYAMLSource creates a new YAML source
func NewYAMLSource(path string) *YAMLSource {
	return &YAMLSource{path: path}
}

// Load loads configuration from YAML file
func (ys *YAMLSource) Load() (map[string]interface{}, error) {
	data, err := os.ReadFile(ys.path)
	if err != nil {
		return nil, fmt.Errorf("failed to read YAML file %s: %w", ys.path, err)
	}

	var config map[string]interface{}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML file %s: %w", ys.path, err)
	}

	// Flatten nested structure for easier merging
	return flattenConfig(config), nil
}

// EnvSource loads configuration from environment variables
type EnvSource struct {
	prefix string
}

// NewEnvSource creates a new environment variable source
func NewEnvSource(prefix string) *EnvSource {
	return &EnvSource{prefix: prefix}
}

// Load loads configuration from environment variables
func (es *EnvSource) Load() (map[string]interface{}, error) {
	config := make(map[string]interface{})

	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := parts[0]
		value := parts[1]

		// Apply prefix filter if specified
		if es.prefix != "" {
			if !strings.HasPrefix(key, es.prefix) {
				continue
			}
			key = strings.TrimPrefix(key, es.prefix)
		}

		// Convert env key to config key (APP__DATABASE__HOST -> AppDatabaseHost)
		configKey := envKeyToConfigKey(key)
		config[configKey] = value
	}

	return config, nil
}

// envKeyToConfigKey converts environment variable key to config key
func envKeyToConfigKey(envKey string) string {
	parts := strings.Split(strings.ToLower(envKey), "__")
	
	var result []string
	for _, part := range parts {
		if part != "" {
			// Convert to PascalCase: database_host -> DatabaseHost
			words := strings.Split(part, "_")
			for _, word := range words {
				if word != "" {
					result = append(result, strings.Title(word))
				}
			}
		}
	}
	
	return strings.Join(result, "")
}

// flattenConfig flattens nested configuration structure
func flattenConfig(config map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	
	flatten(config, "", result)
	
	return result
}

func flatten(current interface{}, prefix string, result map[string]interface{}) {
	switch cv := current.(type) {
	case map[string]interface{}:
		for key, value := range cv {
			newPrefix := ""
			if prefix != "" {
				newPrefix = prefix + strings.Title(key)
			} else {
				newPrefix = strings.Title(key)
			}
			flatten(value, newPrefix, result)
		}
	default:
		if prefix != "" {
			result[prefix] = current
		}
	}
}
