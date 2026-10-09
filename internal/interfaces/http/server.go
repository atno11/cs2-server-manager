package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

const DefaultAddress = "127.0.0.1:8080"

// Serve starts the HTTP API and supports graceful shutdown.
func Serve(ctx context.Context, address string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("start HTTP listener: %w", err)
	}
	defer listener.Close()

	server := &http.Server{
		Handler:           NewHandler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	serveDone := make(chan struct{})
	shutdownDone := make(chan error, 1)

	go func() {
		select {
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(
				context.Background(),
				5*time.Second,
			)
			defer cancel()

			shutdownDone <- server.Shutdown(shutdownCtx)

		case <-serveDone:
			shutdownDone <- nil
		}
	}()

	err = server.Serve(listener)
	close(serveDone)

	shutdownErr := <-shutdownDone
	if shutdownErr != nil {
		return fmt.Errorf(
			"shut down HTTP API: %w",
			shutdownErr,
		)
	}

	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("serve HTTP API: %w", err)
	}

	return nil
}
