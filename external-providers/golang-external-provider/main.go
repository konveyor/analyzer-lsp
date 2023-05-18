package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/bombsimon/logrusr/v3"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor/golang-external-provider/pkg/golang"
	"github.com/sirupsen/logrus"
)

var (
	port = flag.Int("port", 0, "Port must be set")
)

func main() {
	flag.Parse()
	logrusLog := logrus.New()
	logrusLog.SetOutput(os.Stdout)
	logrusLog.SetFormatter(&logrus.TextFormatter{})
	// need to do research on mapping in logrusr to level here TODO
	logrusLog.SetLevel(logrus.Level(5))

	log := logrusr.New(logrusLog)

	client := golang.NewGolangProvider()

	if port == nil || *port == 0 {
		panic(fmt.Errorf("must pass in the port for the external provider"))
	}

	s := provider.NewServer(client, *port, log)
	ctx := context.TODO()
	s.Start(ctx)
}
