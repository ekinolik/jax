package version

import "fmt"

// Version is set at link time via -ldflags "-X .../internal/version.Version=...".
// Local builds without ldflags report "dev".
var Version = "dev"

// Line returns the version string printed by `jax --version`.
func Line() string {
	return fmt.Sprintf("jax %s", Version)
}
