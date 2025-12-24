package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"
	"github.com/zinrai/savalet/internal/api"
	"github.com/zinrai/savalet/internal/config"
)

var (
	apiConfigFile string
	apiSocketPath string
	apiListenAddr string
	apiTimeout    int
	apiLogLevel   string
)

var apiCmd = &cobra.Command{
	Use:   "api",
	Short: "Start HTTP API server",
	Long: `Start the savalet API server that receives HTTP requests and forwards them
to the mediator daemon via Unix domain socket.`,
	Example: `  savalet api --listen :8080 --socket /var/run/savalet.sock
  savalet api --config /etc/savalet/api.yaml`,
	PreRunE: validateAPIFlags,
	RunE:    runAPI,
}

func init() {
	apiCmd.Flags().StringVarP(&apiConfigFile, "config", "c", "/etc/savalet/api.yaml", "Configuration file path")
	apiCmd.Flags().StringVarP(&apiSocketPath, "socket", "s", "", "Unix domain socket path to connect to daemon (overrides config)")
	apiCmd.Flags().StringVarP(&apiListenAddr, "listen", "l", "", "HTTP listen address (overrides config)")
	apiCmd.Flags().IntVarP(&apiTimeout, "timeout", "t", 30, "Default request timeout in seconds")
	apiCmd.Flags().StringVar(&apiLogLevel, "log-level", "info", "Log level (debug|info|warn|error)")
}

func validateAPIFlags(cmd *cobra.Command, args []string) error {
	// Config file is optional for API mode
	if apiConfigFile != "" {
		if _, err := os.Stat(apiConfigFile); os.IsNotExist(err) {
			// Config file doesn't exist, but we can continue with flags
			log.Printf("Configuration file not found: %s, using command line flags", apiConfigFile)
			apiConfigFile = ""
		}
	}

	// Validate log level
	switch apiLogLevel {
	case "debug", "info", "warn", "error":
		// valid
	default:
		return fmt.Errorf("invalid log level: %s", apiLogLevel)
	}

	// Validate timeout
	if apiTimeout <= 0 {
		return fmt.Errorf("timeout must be positive: %d", apiTimeout)
	}

	return nil
}

func runAPI(cmd *cobra.Command, args []string) error {
	var cfg *config.APIConfig
	var err error

	// Load configuration if file exists
	if apiConfigFile != "" {
		cfg, err = config.LoadAPIConfig(apiConfigFile)
		if err != nil {
			return fmt.Errorf("failed to load configuration: %w", err)
		}
	} else {
		// Use default configuration
		cfg = &config.APIConfig{
			ListenAddress:  ":8080",
			SocketPath:     "/var/run/savalet.sock",
			RequestTimeout: 30,
		}
	}

	// Override with command line flags
	if apiSocketPath != "" {
		cfg.SocketPath = apiSocketPath
	}
	if apiListenAddr != "" {
		cfg.ListenAddress = apiListenAddr
	}
	if apiTimeout != 30 {
		cfg.RequestTimeout = apiTimeout
	}
	cfg.LogLevel = apiLogLevel

	log.Printf("Starting savalet API server (version: %s)", version)
	log.Printf("Listen address: %s", cfg.ListenAddress)
	log.Printf("Socket path: %s", cfg.SocketPath)
	log.Printf("Default timeout: %d seconds", cfg.RequestTimeout)
	log.Printf("Log level: %s", cfg.LogLevel)

	// Start API server
	apiServer := api.New(cfg)
	return apiServer.Start()
}
