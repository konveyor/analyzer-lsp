package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/bombsimon/logrusr/v3"
	java "github.com/konveyor/analyzer-lsp/external-providers/java-external-provider/pkg/java_external_provider"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/sirupsen/logrus"
)

var (
	port          = flag.Int("port", 0, "Port must be set")
	socket        = flag.String("socket", "", "Socket to be used")
	logLevel      = flag.Int("log-level", 5, "Level to log")
	lspServerName = flag.String("name", "java", "name of the lsp to be used in rules")
	contextLines  = flag.Int("contxtLines", 10, "lines of context for the code snippet")
	certFile      = flag.String("certFile", "", "Path to the cert file")
	keyFile       = flag.String("keyFile", "", "Path to the key file")
	secretKey     = flag.String("secretKey", "", "Secret Key value")
)

func main() {
	flag.Parse()

	logrusLog := logrus.New()
	logrusLog.SetOutput(os.Stdout)
	logrusLog.SetFormatter(&logrus.TextFormatter{})
	if logLevel != nil {
		logrusLog.SetLevel(logrus.Level(*logLevel))
	}
	log := logrusr.New(logrusLog)
	log = log.WithName("java-provider")

	// must use lspServerName for use of multiple grpc providers
	client := java.NewJavaProvider(log, *lspServerName, *contextLines, provider.Config{})

	if logLevel != nil && *logLevel != 5 {
		logrusLog.SetLevel(logrus.Level(*logLevel))
	}
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
