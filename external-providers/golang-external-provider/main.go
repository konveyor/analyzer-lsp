package main

import (
	"context"
	"os"

	"github.com/bombsimon/logrusr/v3"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor/golang-external-provider/pkg/golang"
	"github.com/sirupsen/logrus"
)

func main() {
	logrusLog := logrus.New()
	logrusLog.SetOutput(os.Stdout)
	logrusLog.SetFormatter(&logrus.TextFormatter{})
	// need to do research on mapping in logrusr to level here TODO
	logrusLog.SetLevel(logrus.Level(5))

	log := logrusr.New(logrusLog)

	client := golang.NewGolangProvider()

	s := provider.NewServer(client, 17902, log)
	ctx := context.TODO()
	s.Start(ctx)
}
