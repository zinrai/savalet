package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	version = "0.2.0" // Set during build with -ldflags
)

var rootCmd = &cobra.Command{
	Use:   "sevalet",
	Short: "Secure command execution service",
	Long: `Sevalet is a secure command execution service that provides controlled
access to system commands via HTTP API or direct daemon mode.`,
	Version: version,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	// Disable default completion command
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	// Add subcommands
	rootCmd.AddCommand(daemonCmd)
	rootCmd.AddCommand(apiCmd)
}
