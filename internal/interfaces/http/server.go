package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"cserver/internal/service"
)

const DefaultAddress = "127.0.0.1:8080"

// Serve preserves the original Stage 1 entry point.
func Serve(ctx context.Context, address string) error {
	return ServeWithDiscovery(ctx, address, nil)
}

// ServeWithDiscovery starts the local API with a shared
// read-only discovery service and graceful shutdown.
func ServeWithDiscovery(
	ctx context.Context,
	address string,
	discovery *service.ServerDiscoveryService,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("start HTTP listener: %w", err)
	}
	defer listener.Close()

	server := &http.Server{
		Handler:           NewHandlerWithDiscovery(discovery),
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
