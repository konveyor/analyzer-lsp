package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/bombsimon/logrusr/v3"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor/analyzer-lsp/provider/lib"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

var (
	providerSettings = flag.String("provider-settings", "provider_settings.json", "path to provider settings file")
	treeOutput       = flag.Bool("tree", false, "output dependencies as a tree")
	outputFile       = flag.String("output-file", "output.yaml", "path to output file")
)

type DepsFlatItem struct {
	Provider     string         `yaml:"Provider"`
	Dependencies []provider.Dep `yaml:"Dependencies"`
}

type DepsTreeItem struct {
	Provider     string                          `yaml:"Provider"`
	Dependencies map[provider.Dep][]provider.Dep `yaml:"Dependencies"`
}

func main() {
	logrusLog := logrus.New()
	logrusLog.SetOutput(os.Stdout)
	logrusLog.SetFormatter(&logrus.TextFormatter{})
	log := logrusr.New(logrusLog)

	flag.Parse()

	err := validateFlags()
	if err != nil {
		log.Error(err, "failed to validate input flags")
		os.Exit(1)
	}

	providers := map[string]provider.Client{}

	// Get the configs
	configs, err := provider.GetConfig(*providerSettings)
	if err != nil {
		log.Error(err, "unable to get configuration")
		os.Exit(1)
	}

	for _, config := range configs {
		provider, err := lib.GetProviderClient(config, log)
		if err != nil {
			log.Error(err, "unable to create provider client")
			os.Exit(1)
		}
		providers[config.Name] = provider
	}

	var depsFlat []DepsFlatItem
	var depsTree []DepsTreeItem
	for name, prov := range providers {
		if !prov.HasCapability("dependency") {
			log.Info("provider does not have dependency capability", "provider", name)
			continue
		}

		if *treeOutput {
			providerDeps := DepsTreeItem{
				Provider:     name,
				Dependencies: make(map[provider.Dep][]provider.Dep),
			}
			deps, _, err := prov.GetDependenciesLinkedList()
			if err != nil {
				log.Error(err, "failed to get list of dependencies for provider", "provider", name)
				continue
			}
			for parentDep, transitiveDeps := range deps {
				providerDeps.Dependencies[parentDep] = transitiveDeps
			}
			depsTree = append(depsTree, providerDeps)
		} else {
			providerDeps := DepsFlatItem{
				Provider:     name,
				Dependencies: make([]provider.Dep, 0),
			}
			deps, _, err := prov.GetDependencies()
			if err != nil {
				log.Error(err, "failed to get list of dependencies for provider", "provider", name)
				continue
			}
			providerDeps.Dependencies = append(providerDeps.Dependencies, deps...)
			depsFlat = append(depsFlat, providerDeps)
		}

	}

	if depsFlat == nil && depsTree == nil {
		log.Info("failed to get dependencies from all given providers")
		os.Exit(1)
	}

	var b []byte
	if *treeOutput {
		b, err = yaml.Marshal(depsTree)
		if err != nil {
			log.Error(err, "failed to marshal dependency data as yaml")
			os.Exit(1)
		}
	} else {
		b, err = yaml.Marshal(depsFlat)
		if err != nil {
			log.Error(err, "failed to marshal dependency data as yaml")
			os.Exit(1)
		}
	}

	fmt.Printf("%s", string(b))

	err = os.WriteFile(*outputFile, b, 0644)
	if err != nil {
		log.Error(err, "failed to write dependencies to output file", "file", *outputFile)
		os.Exit(1)
	}
}

func validateFlags() error {
	_, err := os.Stat(*providerSettings)
	if err != nil {
		return fmt.Errorf("unable to find provider settings file")
	}

	return nil
}
