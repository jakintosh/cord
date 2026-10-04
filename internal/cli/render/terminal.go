package render

import "os"

// StyleEnabled follows the CLI's terminal and NO_COLOR convention.
func StyleEnabled(
	file *os.File,
) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
