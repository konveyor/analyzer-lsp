package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/bombsimon/logrusr/v3"
	pythonprov "github.com/konveyor/analyzer-lsp/external-providers/python-external-provider/pkg/python_external_provider"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/sirupsen/logrus"
)

var (
	port          = flag.Int("port", 0, "Port must be set")
	socket        = flag.String("socket", "", "Socket to be used")
	logLevel      = flag.Int("log-level", 5, "Level to log")
	lspServerName = flag.String("name", "python", "LSP server name advertised to the analyzer")
	certFile      = flag.String("certFile", "", "Path to the cert file")
	keyFile       = flag.String("keyFile", "", "Path to the key file")
	secretKey     = flag.String("secretKey", "", "Secret Key value")
)

func main() {
	flag.Parse()

	logrusLog := logrus.New()
	logrusLog.SetOutput(os.Stdout)
	logrusLog.SetFormatter(&logrus.TextFormatter{})
	logrusLog.SetLevel(logrus.Level(*logLevel))
	log := logrusr.New(logrusLog).WithName("python-external-provider")

	client := pythonprov.NewPythonProvider(*lspServerName, log, nil)

	if *socket == "" && *port == 0 {
		log.Error(fmt.Errorf("no serving location"), "port or socket must be set.")
		os.Exit(1)
	}

	c := *certFile
	k := *keyFile
	secret := *secretKey

	s := provider.NewServer(client, *port, c, k, secret, *socket, log)
	ctx := context.TODO()
	if err := s.Start(ctx); err != nil {
		log.Error(err, "server exited")
		os.Exit(1)
	}
}
