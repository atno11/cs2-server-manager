package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"cserver/internal/app"
	httpapi "cserver/internal/interfaces/http"
	"cserver/internal/interfaces/tui"
	"cserver/internal/service"
)

const Version = service.Version

// Exit codes:
// 0: success
// 1: execution or configuration error
// 2: invalid command usage

// Run executes the CLI and returns an exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printHelp(stdout)
		return 0
	}

	switch args[0] {
	case "help", "-h", "--help":
		if len(args) != 1 {
			return invalidUsage(stderr)
		}

		printHelp(stdout)
		return 0

	case "version", "--version":
		if len(args) != 1 {
			return invalidUsage(stderr)
		}

		info := service.GetApplicationInfo()
		fmt.Fprintln(stdout, info.Name, info.Version)
		return 0

	case "config":
		if len(args) != 2 || args[1] != "show" {
			fmt.Fprintln(stderr, "Usage: cserver config show")
			return 2
		}

		return showConfig(stdout, stderr)

	case "api":
		if len(args) != 2 || args[1] != "serve" {
			fmt.Fprintln(stderr, "Usage: cserver api serve")
			return 2
		}

		return serveAPI(stderr)

	case "tui":
		if len(args) != 1 {
			fmt.Fprintln(stderr, "Usage: cserver tui")
			return 2
		}

		return runTUI(stdout, stderr)

	default:
		fmt.Fprintf(
			stderr,
			"Unknown command: %s\n",
			args[0],
		)
		return 2
	}
}

func initializeApplication(stderr io.Writer) (app.App, bool) {
	application, err := app.New()
	if err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return app.App{}, false
	}

	return application, true
}

func showConfig(stdout, stderr io.Writer) int {
	application, ok := initializeApplication(stderr)
	if !ok {
		return 1
	}

	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(application.Config); err != nil {
		fmt.Fprintln(
			stderr,
			"Failed to display configuration:",
			err,
		)
		return 1
	}

	return 0
}

func runTUI(stdout, stderr io.Writer) int {
	application, ok := initializeApplication(stderr)
	if !ok {
		return 1
	}

	if err := tui.Run(application, stdout); err != nil {
		fmt.Fprintln(
			stderr,
			"Failed to run TUI:",
			err,
		)
		return 1
	}

	return 0
}

func serveAPI(stderr io.Writer) int {
	application, ok := initializeApplication(stderr)
	if !ok {
		return 1
	}

	logger, err := app.NewLogger(
		application.Config,
		stderr,
	)
	if err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 1
	}

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	address := application.Config.HTTPAddress

	logger.Info(
		"starting HTTP API",
		"address", address,
	)

	if err := httpapi.Serve(ctx, address); err != nil {
		logger.Error(
			"HTTP API failed",
			"error", err,
		)
		return 1
	}

	logger.Info("HTTP API stopped")

	return 0
}

func invalidUsage(stderr io.Writer) int {
	fmt.Fprintln(stderr, "Invalid command usage")
	return 2
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, `CServer Manager
A modular Counter-Strike 2 dedicated server manager.

Usage:
  cserver <command>

Commands:
  help           Show this help message
  version        Show application version
  config show    Display the current configuration
  api serve      Start the local HTTP API
  tui            Start the interactive terminal interface

Environment Variables:
  CSERVER_ROOT           Root directory containing CS2 instances
  CSERVER_HTTP_ADDRESS   HTTP listener address (127.0.0.1:8080)
  CSERVER_LOG_LEVEL      debug, info, warn, error
  CSERVER_LOG_FORMAT     text, json

HTTP API:
  Default address: 127.0.0.1:8080
  GET /healthz
  GET /api/v1/info

Server management operations are not yet available.`)
}
