package config

import (
	"fmt"
	"os"

	"github.com/zinrai/savalet/internal/models"
	"gopkg.in/yaml.v3"
)

// DaemonConfig represents the daemon mode configuration
type DaemonConfig struct {
	SocketPath        string             `yaml:"socket_path"`
	SocketPermissions string             `yaml:"socket_permissions"`
	MaxExecutionTime  int                `yaml:"max_execution_time"`
	DefaultTimeout    int                `yaml:"default_timeout"`
	Commands          models.CommandList `yaml:",inline"`
	LogLevel          string             `yaml:"-"` // Set via command line only
}

// APIConfig represents the API mode configuration
type APIConfig struct {
	ListenAddress  string `yaml:"listen_address"`
	SocketPath     string `yaml:"socket_path"`
	RequestTimeout int    `yaml:"request_timeout"`
	MaxBodySize    int    `yaml:"max_body_size"`
	LogLevel       string `yaml:"-"` // Set via command line only
}

// LoadDaemonConfig loads the daemon configuration from a YAML file
func LoadDaemonConfig(path string) (*DaemonConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config DaemonConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Set defaults
	if config.SocketPath == "" {
		config.SocketPath = "/var/run/savalet.sock"
	}
	if config.SocketPermissions == "" {
		config.SocketPermissions = "0660"
	}
	if config.MaxExecutionTime <= 0 {
		config.MaxExecutionTime = 300
	}
	if config.DefaultTimeout <= 0 {
		config.DefaultTimeout = 30
	}

	// Validate commands
	if len(config.Commands.Commands) == 0 {
		return nil, fmt.Errorf("no commands defined in configuration")
	}

	return &config, nil
}

// LoadAPIConfig loads the API configuration from a YAML file
func LoadAPIConfig(path string) (*APIConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config APIConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Set defaults
	if config.ListenAddress == "" {
		config.ListenAddress = ":8080"
	}
	if config.SocketPath == "" {
		config.SocketPath = "/var/run/savalet.sock"
	}
	if config.RequestTimeout <= 0 {
		config.RequestTimeout = 60
	}
	if config.MaxBodySize <= 0 {
		config.MaxBodySize = 1048576 // 1MB
	}

	return &config, nil
}
