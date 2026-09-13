package resource

// Recovery tells an owner how to respond to a resource's failure.
// It describes recovery at the reporting resource's boundary, not its children.
type Recovery uint8

const (
	Retry   Recovery = iota // The resource remains operational and will retry the work.
	Restart                 // The owner must restore the resource's lifetime.
	Fatal                   // Continuing with the current configuration is not viable.
)

// Error classifies a failure while preserving its underlying causes.
// Classify a whole operation before returning it; joined child failures do not
// independently determine how the parent should recover this resource.
type Error struct {
	Recovery Recovery
	Cause    error
}

func (e *Error) Error() string {
	return e.Cause.Error()
}

func (e *Error) Unwrap() error {
	return e.Cause
}
