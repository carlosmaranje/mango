package constants

import (
	"os"
	"path/filepath"
)

const AppName = "mango"

// MangoDir returns the base directory for all mango runtime files.
// Override with MANGO_DIR; defaults to ~/.mango.
func MangoDir() string {
	if dir := os.Getenv("MANGO_DIR"); dir != "" {
		return dir
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, "."+AppName)
	}
	return "/etc/" + AppName
}
