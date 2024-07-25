package main

import (
	"log"
	"os"

	"github.com/dave/dst"
	"github.com/dave/dst/decorator"
	"github.com/dave/dst/dstutil"
	"golang.org/x/tools/go/packages"
)

// InstrumentationFunc is a type that is invoked on a function declaration
type InstrumentationFunc func(n dst.Node, data *InstrumentationManager, c *dstutil.Cursor)

// apply instrumentation to the package
func instrumentPackage(data *InstrumentationManager, instrumentationFunctions ...InstrumentationFunc) {
	for _, file := range data.pkg.Syntax {
		for _, d := range file.Decls {
			if fn, isFn := d.(*dst.FuncDecl); isFn {
				dstutil.Apply(fn, nil, func(c *dstutil.Cursor) bool {
					n := c.Node()
					for _, instFunc := range instrumentationFunctions {
						instFunc(n, data, c)
					}
					return true
				})
			}
		}
	}
}

// traceFunctionCalls discovers and sets up tracing for all function calls in the current package
func tracePackageFunctionCalls(data *InstrumentationManager) {
	files := data.pkg.Syntax
	for _, file := range files {
		for _, decl := range file.Decls {
			if fn, isFn := decl.(*dst.FuncDecl); isFn {
				data.CreateFunctionDeclaration(fn)
			}
		}
	}
}

func discoveredMain(data *InstrumentationManager) bool {
	for _, fn := range data.tracedFuncs {
		if fn.body.Name.Name == "main" {
			return true
		}
	}
	return false
}

func InstrumentPackage(pkg *decorator.Package, appName, agentVariableName, diffFile string) {
	data := NewInstrumentationManager(pkg, appName, agentVariableName, diffFile)

	// Create a call graph of all calls made to functions in this package
	tracePackageFunctionCalls(data)

	if !discoveredMain(data) {
		log.Fatalf("Could not find main function in package %s; this application can not be instrumented", pkg.PkgPath)
	}

	// Instrumentation Steps
	// 	- import the agent
	//	- initialize the agent
	//	- shutdown the agent
	instrumentPackage(data, InstrumentMain, InstrumentHandleFunction, InstrumentHttpClient, CannotInstrumentHttpMethod)

	data.AddRequiredModules()
	data.WriteDiff()
}

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

	for _, pkg := range pkgs {
		InstrumentPackage(pkg, cfg.AppName, cfg.AgentVariableName, cfg.DiffFile)
	}
}
