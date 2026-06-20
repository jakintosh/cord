package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
	"git.studiopollinator.com/pollinator/cord/internal/daemon"
)

var daemonCmd = &args.Command{
	Name: "daemon",
	Help: "run the cord daemon process",
	Handler: func(i *args.Input) error {
		socketPath := i.GetParameterOr("socket-path", DEFAULT_SOCK)

		mux := http.NewServeMux()
		mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		})

		d, err := daemon.New(socketPath, mux)
		if err != nil {
			return err
		}

		ctx, cancel := signal.NotifyContext(
			context.Background(), os.Interrupt, syscall.SIGTERM,
		)
		defer cancel()

		return d.Run(ctx)
	},
}
