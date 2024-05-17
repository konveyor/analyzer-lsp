package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/bombsimon/logrusr/v3"
	"github.com/konveyor/analyzer-lsp/provider"
	java "github.com/konveyor/java-external-provider/pkg/java_external_provider"
	"github.com/sirupsen/logrus"
)

var (
	port          = flag.Int("port", 0, "Port must be set")
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
	logrusLog.SetLevel(logrus.Level(5))
	log := logrusr.New(logrusLog)

	// must use lspServerName for use of multiple grpc providers
	client := java.NewJavaProvider(log, *lspServerName, *contextLines)

	if logLevel != nil && *logLevel != 5 {
		logrusLog.SetLevel(logrus.Level(*logLevel))
	}
	if port == nil || *port == 0 {
		log.Error(fmt.Errorf("port unspecified"), "port number must be specified")
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

	s := provider.NewServer(client, *port, c, k, secret, log)
	ctx := context.TODO()
	s.Start(ctx)
}
