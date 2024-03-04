package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/bombsimon/logrusr/v3"
	"github.com/konveyor/analyzer-lsp/engine/labels"
	"github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor/analyzer-lsp/provider/lib"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

var (
	providerSettings string
	treeOutput       bool
	outputFile       string
	depLabelSelector string

	rootCmd = &cobra.Command{
		Use:   "analyze",
		Short: "Tool for working with analyzer-lsp",
		Run:   func(c *cobra.Command, args []string) {},
	}
)

func init() {
	rootCmd.Flags().StringVar(&providerSettings, "provider-settings", "provider_settings.json", "path to the provider settings")
	rootCmd.Flags().BoolVar(&treeOutput, "tree", false, "output dependencies as a tree")
	rootCmd.Flags().StringVar(&outputFile, "output-file", "output.yaml", "path to output file")
	rootCmd.Flags().StringVar(&depLabelSelector, "dep-label-selector", "", "an expression to select dependencies based on labels provided by the provider")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		println(err.Error())
	}

	logrusLog := logrus.New()
	logrusLog.SetOutput(os.Stdout)
	logrusLog.SetFormatter(&logrus.TextFormatter{})
	log := logrusr.New(logrusLog)

	logrusErrLog := logrus.New()
	logrusErrLog.SetOutput(os.Stderr)
	errLog := logrusr.New(logrusErrLog)

	err := validateFlags()
	if err != nil {
		errLog.Error(err, "failed to validate input flags")
		os.Exit(1)
	}

	var labelSelector *labels.LabelSelector[*konveyor.Dep]
	if depLabelSelector != "" {
		labelSelector, err = labels.NewLabelSelector[*konveyor.Dep](depLabelSelector, nil)
		if err != nil {
			errLog.Error(err, "invalid label selector")
			os.Exit(1)
		}
	}

	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()

	providers := map[string]provider.Client{}

	// Get the configs
	configs, err := provider.GetConfig(providerSettings)
	if err != nil {
		errLog.Error(err, "unable to get configuration")
		os.Exit(1)
	}

	for _, config := range configs {
		prov, err := lib.GetProviderClient(config, log)
		if err != nil {
			errLog.Error(err, "unable to create provider client")
			os.Exit(1)
		}
		if s, ok := prov.(provider.Startable); ok {
			if err := s.Start(ctx); err != nil {
				errLog.Error(err, "unable to create provider client")
				os.Exit(1)
			}
		}

		time.Sleep(5 * time.Second)

		err = prov.ProviderInit(ctx)
		b, _ := json.Marshal(config)
		if err != nil {
			errLog.Error(err, "unable to init the providers", "provider", config.Name, "the-error-is", err, "config", string(b))
			os.Exit(1)
		} else {
			log.Info("init'd provider", "provider", config.Name, "config", string(b))
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

		if treeOutput {
			deps, err := prov.GetDependenciesDAG(ctx)
			if err != nil {
				errLog.Error(err, "failed to get list of dependencies for provider", "provider", name)
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
			deps, err := prov.GetDependencies(ctx)
			if err != nil {
				errLog.Error(err, "failed to get list of dependencies for provider", "provider", name)
				continue
			}
			for u, ds := range deps {
				newDeps := ds
				if labelSelector != nil {
					newDeps, err = labelSelector.MatchList(ds)
					if err != nil {
						errLog.Error(err, "error matching label selector on deps")
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

	// stop providers before exiting
	for _, prov := range providers {
		prov.Stop()
	}

	if depsFlat == nil && depsTree == nil {
		errLog.Info("failed to get dependencies from all given providers")
		os.Exit(1)
	}

	var b []byte
	if treeOutput {
		b, err = yaml.Marshal(depsTree)
		if err != nil {
			errLog.Error(err, "failed to marshal dependency data as yaml")
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
			errLog.Error(err, "failed to marshal dependency data as yaml")
			os.Exit(1)
		}
	}

	err = os.WriteFile(outputFile, b, 0644)
	if err != nil {
		errLog.Error(err, "failed to write dependencies to output file", "file", outputFile)
		os.Exit(1)
	}
}

func validateFlags() error {
	_, err := os.Stat(providerSettings)
	if err != nil {
		return fmt.Errorf("unable to find provider settings file")
	}

	return nil
}
