package backend

import (
	"fmt"
	"os"
)

// GetXDGRuntimeDir returns the XDG_RUNTIME_DIR for the current user.
// It first checks the XDG_RUNTIME_DIR environment variable, and if not set,
// falls back to the standard /run/user/{uid} path.
func GetXDGRuntimeDir() string {
	if xdgRuntimeDir, ok := os.LookupEnv("XDG_RUNTIME_DIR"); ok {
		return xdgRuntimeDir
	}
	return fmt.Sprintf("/run/user/%d", os.Getuid())
}
