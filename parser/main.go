package main

import (
	"log"
	"os"

	"github.com/dave/dst/decorator"
	"golang.org/x/tools/go/packages"
)

func createDiffFile(path string) {
	f, err := os.Create(path)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
}

func main() {
	cfg := NewCLIConfig()

	createDiffFile(cfg.DiffFile)

	loadMode := packages.LoadSyntax
	pkgs, err := decorator.Load(&packages.Config{Dir: cfg.PackagePath, Mode: loadMode}, cfg.PackageName)
	if err != nil {
		log.Fatal(err)
	}

	manager := NewInstrumentationManager(pkgs, cfg.AppName, cfg.AgentVariableName, cfg.DiffFile)
	err = manager.InstrumentPackages()
	if err != nil {
		log.Fatal(err)
	}

	manager.AddRequiredModules()
	manager.WriteDiff()
}
