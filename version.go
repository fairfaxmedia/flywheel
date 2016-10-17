package flywheel

import (
	"bytes"
	"fmt"
)

// GitCommit - The git commit that was compiled. This will be filled in by the compiler.
var GitCommit string

// Version should be overwritten via Makefile
const Version = "0.2.0"

// VersionInfo stores version and GitCommit SHA
type VersionInfo struct {
	Revision string
	Version  string
}

// GetVersion retrieve version
func GetVersion() *VersionInfo {

	return &VersionInfo{
		Revision: GitCommit,
		Version:  Version,
	}
}

func (c *VersionInfo) String() string {
	var versionString bytes.Buffer

	fmt.Fprintf(&versionString, "Flywheel v%s", c.Version)

	if c.Revision != "" {
		fmt.Fprintf(&versionString, " (%s)", c.Revision)
	}

	return versionString.String()
}
