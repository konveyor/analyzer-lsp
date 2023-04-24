package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/bombsimon/logrusr/v3"
	"github.com/konveyor/analyzer-lsp/dependency/dependency"
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
	configs, err := lib.GetConfig(*providerSettings)
	if err != nil {
		log.Error(err, "unable to get configuration")
		os.Exit(1)
	}

	for _, config := range configs {
		provider, err := provider.GetProviderClient(config)
		if err != nil {
			log.Error(err, "unable to create provider client")
			os.Exit(1)
		}
		providers[config.Name] = provider
	}

	var depsFlat []dependency.Dep
	var depsTree map[dependency.Dep][]dependency.Dep
	for name, provider := range providers {
		if !provider.HasCapability("dependency") {
			log.Info("provider does not have dependency capability", "provider", name)
			continue
		}

		if *treeOutput {
			deps, err := provider.GetDependenciesLinkedList()
			if err != nil {
				log.Error(err, "failed to get list of dependencies for provider", "provider", name)
				continue
			}
			if depsTree == nil {
				depsTree = make(map[dependency.Dep][]dependency.Dep)
			}
			for parentDep, transitiveDeps := range deps {
				depsTree[parentDep] = transitiveDeps
			}
		} else {
			deps, err := provider.GetDependencies()
			if err != nil {
				log.Error(err, "failed to get list of dependencies for provider", "provider", name)
				continue
			}
			if depsFlat == nil {
				depsFlat = make([]dependency.Dep, 0)
			}
			depsFlat = append(depsFlat, deps...)
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
