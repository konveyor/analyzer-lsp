package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/bombsimon/logrusr/v3"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor/generic-external-provider/pkg/generic_external_provider"
	"github.com/sirupsen/logrus"
)

var (
	port          = flag.Int("port", 0, "Port must be set")
	lspServerName = flag.String("name", "", "lsp server name")
)

func main() {
	flag.Parse()
	logrusLog := logrus.New()
	logrusLog.SetOutput(os.Stdout)
	logrusLog.SetFormatter(&logrus.TextFormatter{})
	// TODO: Need to do research on mapping in logrusr to level here
	logrusLog.SetLevel(logrus.Level(5))

	log := logrusr.New(logrusLog)

	// NOTE(jsussman): The analyzer-lsp checks for advertized capabilities
	// *before* initializing any service clients. Due to the way that capabilities
	// are implemented, we must lock in what lsp server we are using early and
	// spawn multiple different generic-external-providers for each language we
	// want to analyze.
	//
	// For example, "go.referenced" calls the "go" provider, which references a
	// specific provider, and executes the "referenced" capability on one of its
	// service clients. If you wanted to add "python.referenced", we have to spawn
	// a whole new process.
	//
	// We could "fix" this by doing generic requests like
	// "generic.gopls.referenced" or something, but that ruins the whole
	// interchangeability aspect of the providers
	if lspServerName == nil || *lspServerName == "" {
		x := "generic"
		lspServerName = &x
		// panic(fmt.Errorf("must pass in the name of the lsp server"))
	}

	client := generic_external_provider.NewGenericProvider(*lspServerName)

	if port == nil || *port == 0 {
		panic(fmt.Errorf("must pass in the port for the external provider"))
	}

	s := provider.NewServer(client, *port, log)
	ctx := context.TODO()
	s.Start(ctx)
}
