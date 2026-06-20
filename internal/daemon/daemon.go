package daemon

import (
	"context"
	"net"
	"net/http"
	"os"
	"time"
)

type Daemon struct {
	server   *http.Server
	listener net.Listener
}

func New(
	socketPath string,
	handler http.Handler,
) (
	*Daemon,
	error,
) {
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(socketPath, 0700); err != nil {
		listener.Close()
		return nil, err
	}

	return &Daemon{
		server: &http.Server{
			Handler: handler,
		},
		listener: listener,
	}, nil
}

func (d *Daemon) Run(
	ctx context.Context,
) error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- d.server.Serve(d.listener)
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return d.server.Shutdown(shutdownCtx)
}
