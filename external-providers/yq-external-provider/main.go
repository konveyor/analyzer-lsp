package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/bombsimon/logrusr/v3"
	"github.com/konveyor/analyzer-lsp/external-providers/yq-external-provider/pkg/yq_provider"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/sirupsen/logrus"
)

var (
	port      = flag.Int("port", 0, "Port must be set")
	socket    = flag.String("socket", "", "Socket to be used")
	name      = flag.String("name", "yaml", "Port must be set")
	logLevel  = flag.Int("log-level", 5, "Level to log")
	certFile  = flag.String("certFile", "", "Path to the cert file")
	keyFile   = flag.String("keyFile", "", "Path to the key file")
	secretKey = flag.String("secretKey", "", "Secret Key value")
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

	log := logrusr.New(logrusLog).WithName(*name)

	client := yq_provider.NewYqProvider()

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
