package main

import (
	"context"
	"os"

	"github.com/bombsimon/logrusr/v3"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor/analyzer-lsp/provider/lib"
	"github.com/konveyor/golang-external-provider/pkg/golang"
	"github.com/sirupsen/logrus"
)

func main() {
	configs, err := lib.GetConfig("/analyzer-lsp/provider_settings.json")
	if err != nil {
		panic(err)
	}
	var c lib.Config
	for _, config := range configs {
		if config.Name == "go" {
			c = config
		}
	}

	logrusLog := logrus.New()
	logrusLog.SetOutput(os.Stdout)
	logrusLog.SetFormatter(&logrus.TextFormatter{})
	// need to do research on mapping in logrusr to level here TODO
	logrusLog.SetLevel(logrus.Level(5))

	log := logrusr.New(logrusLog)

	client := golang.NewGolangProvider(c)

	s := provider.NewServer(client, 17902, log)
	ctx := context.TODO()
	s.Start(ctx)
}
