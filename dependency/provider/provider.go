package provider

import "github.com/konveyor/analyzer-lsp/dependency/dependency"

type DependencyProvider interface {
	// GetDependencies will get the dependencies
	// It is the responsibility of the provider to determine how that is done
	GetDependencies() ([]dependency.Dep, error)
	// GetDependencies will get the dependencies and return them as a linked list
	// Top level items are direct dependencies, the rest are indirect dependencies
	GetDependenciesLinkedList() (map[dependency.Dep][]dependency.Dep, error)
}
