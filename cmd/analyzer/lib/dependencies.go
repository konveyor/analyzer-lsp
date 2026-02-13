package lib

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/engine/labels"
	"github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor/analyzer-lsp/provider/lib"
)

// DependencyConfig holds configuration for retrieving dependencies
type DependencyConfig struct {
	ProviderSettings string
	TreeFormat       bool
	LabelSelector    string
}

// GetDependencies retrieves dependencies from all providers
// This is extracted from cmd/dep/main.go to allow reuse
func GetDependencies(ctx context.Context, config DependencyConfig, log logr.Logger) (interface{}, error) {
	// Create label selector if provided
	var labelSelector *labels.LabelSelector[*konveyor.Dep]
	var err error
	if config.LabelSelector != "" {
		labelSelector, err = labels.NewLabelSelector[*konveyor.Dep](config.LabelSelector, nil)
		if err != nil {
			return nil, fmt.Errorf("invalid label selector: %w", err)
		}
	}

	// Get provider configs
	configs, err := provider.GetConfig(config.ProviderSettings)
	if err != nil {
		return nil, fmt.Errorf("unable to get provider configuration: %w", err)
	}

	// Create provider clients
	providers := map[string]provider.Client{}
	for _, provConfig := range configs {
		prov, err := lib.GetProviderClient(provConfig, log)
		if err != nil {
			log.V(5).Info("unable to create provider client", "provider", provConfig.Name, "error", err)
			continue
		}

		// Start provider if it supports starting
		if s, ok := prov.(provider.Startable); ok {
			if err := s.Start(ctx); err != nil {
				log.V(5).Info("unable to start provider", "provider", provConfig.Name, "error", err)
				continue
			}
		}

		// Sleep briefly to allow provider to initialize
		time.Sleep(5 * time.Second)

		// Initialize provider
		_, err = prov.ProviderInit(ctx, nil)
		if err != nil {
			log.V(5).Info("unable to init provider", "provider", provConfig.Name, "error", err)
			prov.Stop()
			continue
		}

		providers[provConfig.Name] = prov
	}

	// Cleanup providers on exit
	defer func() {
		for _, prov := range providers {
			if prov != nil {
				prov.Stop()
			}
		}
	}()

	// Retrieve dependencies
	var depsFlat []konveyor.DepsFlatItem
	var depsTree []konveyor.DepsTreeItem

	for name, prov := range providers {
		// Check if provider has dependency capability
		if !provider.HasCapability(prov.Capabilities(), "dependency") {
			log.V(5).Info("provider does not have dependency capability", "provider", name)
			continue
		}

		if config.TreeFormat {
			// Get dependencies as tree/DAG
			deps, err := prov.GetDependenciesDAG(ctx)
			if err != nil {
				log.V(5).Info("failed to get dependencies for provider", "provider", name, "error", err)
				continue
			}
			for u, ds := range deps {
				depsTree = append(depsTree, konveyor.DepsTreeItem{
					FileURI:      string(u),
					Provider:     name,
					Dependencies: ds,
				})
			}
		} else {
			// Get dependencies as flat list
			deps, err := prov.GetDependencies(ctx)
			if err != nil {
				log.V(5).Info("failed to get dependencies for provider", "provider", name, "error", err)
				continue
			}
			for u, ds := range deps {
				newDeps := ds
				// Apply label selector if provided
				if labelSelector != nil {
					newDeps, err = labelSelector.MatchList(ds)
					if err != nil {
						log.V(5).Info("error matching label selector on deps", "error", err)
						continue
					}
				}
				depsFlat = append(depsFlat, konveyor.DepsFlatItem{
					Provider:     name,
					FileURI:      string(u),
					Dependencies: newDeps,
				})
			}
		}
	}

	// Check if any dependencies were found
	if depsFlat == nil && depsTree == nil {
		return nil, fmt.Errorf("no dependencies found from any providers")
	}

	// Return appropriate format
	if config.TreeFormat {
		return depsTree, nil
	} else {
		// Sort flat dependencies
		sort.SliceStable(depsFlat, func(i, j int) bool {
			if depsFlat[i].Provider == depsFlat[j].Provider {
				return depsFlat[i].FileURI < depsFlat[j].FileURI
			}
			return depsFlat[i].Provider < depsFlat[j].Provider
		})
		return depsFlat, nil
	}
}
