// Package resource defines the shared status vocabulary for owned resources.
package resource

// State describes a resource's current lifecycle condition.
type State string

const (
	Disabled State = "disabled" // The owner has not configured this resource.
	Stopped  State = "stopped"  // Configured, but not started or intentionally stopped.
	Running  State = "running"  // The resource is available.
	Failed   State = "failed"   // Starting, serving, or cleanup failed.
)

// Status is a read-only snapshot. Error explains a failure; State determines
// availability. Owners decide whether and when to attempt recovery.
type Status struct {
	State State
	Error error
}
