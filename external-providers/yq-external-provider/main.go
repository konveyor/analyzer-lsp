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
	name      = flag.String("name", "yaml", "Port must be set")
	certFile  = flag.String("certFile", "", "Path to the cert file")
	keyFile   = flag.String("keyFile", "", "Path to the key file")
	secretKey = flag.String("secretKey", "", "Secret Key value")
)

func main() {
	flag.Parse()
	logrusLog := logrus.New()
	logrusLog.SetOutput(os.Stdout)
	logrusLog.SetFormatter(&logrus.TextFormatter{})
	// need to do research on mapping in logrusr to level here TODO
	logrusLog.SetLevel(logrus.Level(5))

	log := logrusr.New(logrusLog).WithName(*name)

	client := yq_provider.NewYqProvider()

	if port == nil || *port == 0 {
		panic(fmt.Errorf("must pass in the port for the external provider"))
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
