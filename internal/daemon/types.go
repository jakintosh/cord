package daemon

import "time"

type DaemonKind string

const (
	DaemonServer DaemonKind = "server"
	DaemonClient DaemonKind = "client"
)

type DaemonStatus struct {
	Kind     DaemonKind      `json:"kind"`
	Socket   string          `json:"socket"`
	Networks []NetworkStatus `json:"networks"`
}

type NetworkStatus struct {
	Name      string    `json:"name"`
	Enabled   bool      `json:"enabled"`
	Running   bool      `json:"running"`
	Degraded  bool      `json:"degraded"`
	LastSync  time.Time `json:"last_sync"`
	LastError string    `json:"last_error"`
}

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
