package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}
	switch os.Args[1] {
	case "run":
		if err := cmdRun(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "check":
		os.Exit(cmdCheck(os.Args[2:]))
	case "version":
		printVersion()
	case "help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Printf(`savalet - HTTP front door to a fixed set of local commands

Usage:
  savalet <command> [options]

Commands:
  run      Start the server
  check    Validate a configuration file
  help     Show this help message
  version  Show version information

Examples:
  savalet run -config /etc/savalet/savalet.yaml
  savalet check -config /etc/savalet/savalet.yaml

Use "savalet <command> -h" for more information about a command.
`)
}

func cmdRun(args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "Configuration file path")
	fs.Parse(args)

	cfg, err := LoadConfig(*configPath)
	if err != nil {
		return err
	}

	rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	l, err := listen(cfg)
	if err != nil {
		return err
	}

	logger := newAuditLogger(os.Stdout)
	logger.Info("config_loaded",
		"path", *configPath,
		"sha256", cfg.fileSHA256,
		"actions", len(cfg.Actions),
	)

	s := &server{cfg: cfg, logger: logger, rootCtx: rootCtx}
	err = s.serve(l)
	logger.Info("shutdown")
	return err
}

// The executable check on argv[0] binds validation to the host, so check
// cannot stand in for a CI side lint: it belongs on the deploy target,
// typically as the validate step of a configuration push.
func cmdCheck(args []string) int {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "Configuration file path")
	fs.Parse(args)

	data, err := os.ReadFile(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 2
	}
	cfg, err := parseConfig(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", *configPath, err)
		return 1
	}
	fmt.Printf("OK: %d actions\n", len(cfg.Actions))
	return 0
}
