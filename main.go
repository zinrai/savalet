package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/zinrai/savalet/internal/api"
	"github.com/zinrai/savalet/internal/config"
	"github.com/zinrai/savalet/internal/executor"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "api":
		runAPI(os.Args[2:])
	case "executor":
		runExecutor(os.Args[2:])
	case "help":
		printUsage()
	case "version":
		printVersion()
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Printf(`savalet - Secure command execution service

Savalet is a secure command execution service that provides controlled
access to system commands via HTTP API.

Usage:
  savalet <command> [options]

Commands:
  api       Start HTTP API server
  executor  Start command executor
  help      Show this help message
  version   Show version information

Examples:
  savalet api -config /etc/savalet/api.yaml
  savalet executor -config /etc/savalet/executor.yaml

Use "savalet <command> -h" for more information about a command.
`)
}

func runAPI(args []string) {
	fs := flag.NewFlagSet("api", flag.ExitOnError)
	configPath := fs.String("config", "/etc/savalet/api.yaml", "Configuration file path")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Start the savalet API server

Usage:
  savalet api [options]

Options:
`)
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	cfg, err := config.LoadAPIConfig(*configPath)
	if err != nil {
		log.Fatalf("Error: failed to load configuration: %v", err)
	}

	log.Printf("Starting savalet API server (version: %s)", version)
	log.Printf("Listen address: %s", cfg.ListenAddress)
	log.Printf("Socket path: %s", cfg.SocketPath)
	log.Printf("Request timeout: %d seconds", cfg.RequestTimeout)

	apiServer := api.New(cfg)
	if err := apiServer.Start(); err != nil {
		log.Fatalf("Error: %v", err)
	}
}

func runExecutor(args []string) {
	fs := flag.NewFlagSet("executor", flag.ExitOnError)
	configPath := fs.String("config", "/etc/savalet/executor.yaml", "Configuration file path")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Start the savalet executor

Usage:
  savalet executor [options]

Options:
`)
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	cfg, err := config.LoadExecutorConfig(*configPath)
	if err != nil {
		log.Fatalf("Error: failed to load configuration: %v", err)
	}

	log.Printf("Starting savalet executor (version: %s)", version)
	log.Printf("Socket path: %s", cfg.SocketPath)

	e := executor.New(cfg)
	if err := e.Start(); err != nil {
		log.Fatalf("Error: %v", err)
	}
}
