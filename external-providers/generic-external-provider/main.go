package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/bombsimon/logrusr/v3"
	"github.com/konveyor/analyzer-lsp/external-providers/generic-external-provider/pkg/generic_external_provider"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/sirupsen/logrus"
)

var (
	port          = flag.Int("port", 0, "Port must be set")
	socket        = flag.String("socket", "", "Socket to be used")
	lspServerName = flag.String("name", "", "lsp server name")
	logLevel      = flag.Int("log-level", 5, "Level to log")
	certFile      = flag.String("certFile", "", "Path to the cert file")
	keyFile       = flag.String("keyFile", "", "Path to the key file")
	secretKey     = flag.String("secretKey", "", "Secret Key value")
)

func main() {
	flag.Parse()
	logrusLog := logrus.New()
	logrusLog.SetOutput(os.Stdout)
	logrusLog.SetFormatter(&logrus.TextFormatter{})
	// Set log level from flag (default is 5)
	if logLevel != nil {
		logrusLog.SetLevel(logrus.Level(*logLevel))
	} else {
		logrusLog.SetLevel(logrus.Level(5))
	}

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

	client := generic_external_provider.NewGenericProvider(*lspServerName, log, nil)

	if (socket == nil || *socket == "") && (port == nil || *port == 0) {
		log.Error(fmt.Errorf("no serving location"), "port or socket must be set.")
		panic(1)
	}

	var c string
	var k string
	var secret string

	if certFile != nil {
		c = *certFile
	}

	if keyFile != nil {
		k = *keyFile
	}

	if secretKey != nil {
		secret = *secretKey
	}

	s := provider.NewServer(client, *port, c, k, secret, *socket, log)
	ctx := context.TODO()
	s.Start(ctx)
}
