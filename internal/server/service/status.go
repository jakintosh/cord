package service

import "time"

// Status captures the runtime state of a single server network.
type Status struct {
	Name      string
	Enabled   bool
	Running   bool
	Degraded  bool
	LastSync  time.Time
	LastError string
}
