package provider

import "github.com/konveyor/analyzer-lsp/dependency/dependency"

type DependencyProvider interface {
	// GetDependencies will get the depdencies
	// Path is the absolute path to the file to determine deps.
	// In golang this is go.mod, in java pom.xml
	GetDependencies() (map[dependency.Dep][]dependency.Dep, error)
}
