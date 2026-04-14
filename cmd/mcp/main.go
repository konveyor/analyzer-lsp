package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "konveyor-analyzer-mcp",
		Short: "Konveyor Analyzer MCP Server",
		Long:  "Model Context Protocol server that exposes Konveyor static analysis capabilities to AI assistants.",
		RunE: func(cmd *cobra.Command, args []string) error {
			rules, _ := cmd.Flags().GetString("rules")
			providerConfig, _ := cmd.Flags().GetString("provider-config")
			labelSelector, _ := cmd.Flags().GetString("label-selector")
			incidentLimit, _ := cmd.Flags().GetInt("incident-limit")
			codeSnipLimit, _ := cmd.Flags().GetInt("code-snip-limit")
			contextLines, _ := cmd.Flags().GetInt("context-lines")
			transport, _ := cmd.Flags().GetString("transport")
			httpAddr, _ := cmd.Flags().GetString("http-addr")
			verbosity, _ := cmd.Flags().GetInt("verbosity")

			cfg := Config{
				Rules:          rules,
				ProviderConfig: providerConfig,
				LabelSelector:  labelSelector,
				IncidentLimit:  incidentLimit,
				CodeSnipLimit:  codeSnipLimit,
				ContextLines:   contextLines,
				Transport:      transport,
				HTTPAddr:       httpAddr,
				Verbosity:      verbosity,
			}

			return Run(cfg)
		},
	}

	rootCmd.Flags().String("rules", "", "Comma-separated list of rule file/directory paths")
	rootCmd.Flags().String("provider-config", "", "Path to provider configuration file")
	rootCmd.Flags().String("label-selector", "", "Label selector to filter rules (e.g. konveyor.io/target=eap8)")
	rootCmd.Flags().Int("incident-limit", 0, "Maximum incidents per rule (0 = unlimited)")
	rootCmd.Flags().Int("code-snip-limit", 0, "Maximum code snippet characters (0 = unlimited)")
	rootCmd.Flags().Int("context-lines", 10, "Context lines around incidents")
	rootCmd.Flags().String("transport", "stdio", "Transport type: stdio or http")
	rootCmd.Flags().String("http-addr", ":8080", "HTTP listen address (when transport=http)")
	rootCmd.Flags().Int("verbosity", 0, "Log verbosity level")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
