// Package buildinfo exposes build-time metadata.
package buildinfo

const projectName = "nsx-operator"

// ProjectName returns the binary's project name.
func ProjectName() string {
	return projectName
}
