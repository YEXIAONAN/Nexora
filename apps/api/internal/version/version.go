// Package version exposes build metadata injected at link time.
package version

var (
	// Version is the Semantic Version of the running build.
	Version = "0.1.0-alpha.1"
	// Commit is the Git commit SHA the binary was built from.
	Commit = "none"
	// BuildTime is the RFC3339 timestamp of the build.
	BuildTime = "unknown"
)

// Info is the immutable build metadata reported by the API.
type Info struct {
	Version   string
	Commit    string
	BuildTime string
}

// Get returns the build metadata of the current process.
func Get() Info {
	return Info{
		Version:   Version,
		Commit:    Commit,
		BuildTime: BuildTime,
	}
}
