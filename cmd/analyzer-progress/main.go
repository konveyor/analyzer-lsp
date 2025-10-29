package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: analyzer-progress [analyzer arguments]")
		fmt.Println("Example: analyzer-progress --rules examples/rules.yaml --provider-settings provider_settings.json")
		os.Exit(1)
	}

	// Get the analyzer binary path
	analyzerPath := "analyzer"
	if p := os.Getenv("ANALYZER_PATH"); p != "" {
		analyzerPath = p
	}

	// Build arguments with progress flags
	args := os.Args[1:]

	// Add progress flags if not already present
	hasProgress := false
	for _, arg := range args {
		if strings.HasPrefix(arg, "--progress-") {
			hasProgress = true
			break
		}
	}

	if !hasProgress {
		args = append(args, "--progress-output=stderr", "--progress-format=text")
	}

	// Run analyzer with progress
	cmd := exec.Command(analyzerPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	fmt.Fprintf(os.Stderr, "\n=== Running Analyzer with Progress Reporting ===\n\n")

	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			fmt.Fprintf(os.Stderr, "\nAnalysis failed: %v\n", err)
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "\nFailed to execute analyzer: %v\n", err)
		os.Exit(127)
	}

	fmt.Fprintf(os.Stderr, "\n=== Analysis Complete ===\n")
}
