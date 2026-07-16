package main

import (
	"os"

	"github.com/salarrbl/cexss/internal/app"
	"github.com/salarrbl/cexss/internal/config"
	"github.com/salarrbl/cexss/pkg/logger"
)

func main() {
	cfg := config.Parse()

	a := app.New(cfg)

	if err := a.Run(); err != nil {
		logger.Error("%v", err)
		os.Exit(1)
	}
}
