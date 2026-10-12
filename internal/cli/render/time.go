package render

import (
	"fmt"
	"time"
)

// Relative formats an optional timestamp with two-unit precision.
func Relative(
	value *string,
	now time.Time,
	missing string,
) string {
	if value == nil || *value == "" {
		return missing
	}
	at, err := time.Parse(time.RFC3339, *value)
	if err != nil {
		return Clean(*value)
	}
	d := now.Sub(at)
	if d < 0 {
		return "in " + Duration(-d)
	}
	return Duration(d) + " ago"
}

// Duration formats elapsed time using at most two units.
func Duration(
	d time.Duration,
) string {
	d = d.Truncate(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int64(d/time.Second))
	}
	if d < time.Hour {
		if d%time.Minute == 0 {
			return fmt.Sprintf("%dm", int64(d/time.Minute))
		}
		return fmt.Sprintf("%dm %ds", int64(d/time.Minute), int64(d%time.Minute/time.Second))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh %dm", int64(d/time.Hour), int64(d%time.Hour/time.Minute))
	}
	return fmt.Sprintf("%dd %dh", int64(d/(24*time.Hour)), int64(d%(24*time.Hour)/time.Hour))
}

// RelativeTime formats a timestamp with coarse precision for compact lists.
func RelativeTime(
	timestamp string,
	now time.Time,
) string {
	if timestamp == "" {
		return "never"
	}
	t, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return timestamp
	}

	d := now.Sub(t)
	if d == 0 {
		return "now"
	}
	if d < 0 {
		return "in " + coarseDuration(-d)
	}
	return coarseDuration(d) + " ago"
}

func coarseDuration(
	d time.Duration,
) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
