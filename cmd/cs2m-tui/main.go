package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	tea "charm.land/bubbletea/v2"
	"github.com/atno11/cs2-server-manager/internal/apiclient"
	"github.com/atno11/cs2-server-manager/internal/tui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if os.Geteuid() == 0 {
		return errors.New("run as an unprivileged user")
	}
	addr, path, err := configuration(os.Args[1:])
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	token, err := tui.LoadToken(path)
	if err != nil {
		return errors.New("private admin token unavailable; check --token-file or CS2M_DATA")
	}
	client, err := apiclient.New(addr, token)
	if err != nil {
		return errors.New("invalid local API endpoint or credential")
	}
	defer client.CloseIdleConnections()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	m := tui.New(ctx, tui.LiveService{Client: client}, client.URL())
	defer m.Close()
	_, err = tea.NewProgram(m, tea.WithContext(ctx)).Run()
	if errors.Is(err, tea.ErrProgramKilled) && ctx.Err() != nil {
		return nil
	}
	if err != nil {
		return errors.New("terminal session failed")
	}
	return nil
}

func configuration(args []string) (string, string, error) {
	fs := flag.NewFlagSet("cs2m-tui", flag.ContinueOnError)
	addr := fs.String("addr", "http://127.0.0.1:8080", "numeric loopback API endpoint")
	tokenPath := fs.String("token-file", "", "existing private admin.token path")
	if err := fs.Parse(args); err != nil {
		return "", "", err
	}
	if fs.NArg() != 0 {
		return "", "", errors.New("unexpected argument; use --help")
	}
	path := *tokenPath
	if path == "" {
		dir := os.Getenv("CS2M_DATA")
		if dir == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", "", errors.New("home directory unavailable")
			}
			dir = filepath.Join(home, ".local", "share", "cs2m")
		}
		path = filepath.Join(dir, "admin.token")
	}
	return *addr, path, nil
}
