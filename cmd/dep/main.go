package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/bombsimon/logrusr/v3"
	"github.com/konveyor/analyzer-lsp/engine/labels"
	"github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor/analyzer-lsp/provider/lib"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

var (
	providerSettings = flag.String("provider-settings", "provider_settings.json", "path to provider settings file")
	treeOutput       = flag.Bool("tree", false, "output dependencies as a tree")
	outputFile       = flag.String("output-file", "output.yaml", "path to output file")
	depLabelSelector = flag.String("dep-label-selector", "", "an expression to select rules based on labels")
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

	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()

	providers := map[string]provider.Client{}

	// Get the configs
	configs, err := provider.GetConfig(*providerSettings)
	if err != nil {
		log.Error(err, "unable to get configuration")
		os.Exit(1)
	}

	for _, config := range configs {
		prov, err := lib.GetProviderClient(config, log)
		if err != nil {
			log.Error(err, "unable to create provider client")
			os.Exit(1)
		}
		if s, ok := prov.(provider.Startable); ok {
			if err := s.Start(ctx); err != nil {
				log.Error(err, "unable to create provider client")
				os.Exit(1)
			}
		}
		err = prov.ProviderInit(ctx)
		if err != nil {
			log.Error(err, "unable to init the providers", "provider", config.Name)
			os.Exit(1)
		}
		providers[config.Name] = prov

	}

	var depsFlat []konveyor.DepsFlatItem
	var depsTree []konveyor.DepsTreeItem
	for name, prov := range providers {
		if !provider.HasCapability(prov.Capabilities(), "dependency") {
			log.Info("provider does not have dependency capability", "provider", name)
			continue
		}

		if *treeOutput {
			deps, err := prov.GetDependenciesDAG()
			if err != nil {
				log.Error(err, "failed to get list of dependencies for provider", "provider", name)
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
			deps, err := prov.GetDependencies()
			if err != nil {
				log.Error(err, "failed to get list of dependencies for provider", "provider", name)
				continue
			}
			for u, ds := range deps {
				newDeps := ds
				if depLabelSelector != nil && *depLabelSelector != "" {
					l, err := labels.NewLabelSelector[*konveyor.Dep](*depLabelSelector)
					if err != nil {
						panic(err)
					}
					newDeps, err = l.MatchList(ds)
					if err != nil {
						panic(err)
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
		// Sort depsFlat
		sort.SliceStable(depsFlat, func(i, j int) bool {
			if depsFlat[i].Provider == depsFlat[j].Provider {
				return depsFlat[i].FileURI < depsFlat[j].FileURI
			} else {
				return depsFlat[i].Provider < depsFlat[j].Provider
			}
		})

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
