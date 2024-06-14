package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"

	"github.com/dave/dst"
	"github.com/dave/dst/decorator"
	"github.com/dave/dst/dstutil"
	"golang.org/x/tools/go/packages"
)

// Default Values
const (
	defaultAgentVariableName = "NewRelicAgent"
	defaultAppName           = "AST Example"
	defaultPackageName       = "."
	defaultPackagePath       = "../demo-app"
)

type InstrumentationFunc func(n dst.Node, data *InstrumentationManager, c *dstutil.Cursor)

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

func InstrumentPackage(pkg *decorator.Package, appName, agentVariableName, diffFile string) {
	data := NewInstrumentationManager(pkg, appName, agentVariableName, diffFile)

	// Create a call graph of all calls made to functions in this package
	tracePackageFunctionCalls(data)

	// Instrumentation Steps
	// 	- import the agent
	//	- initialize the agent
	//	- shutdown the agent
	instrumentPackage(data, InstrumentMain, InstrumentHandleFunction, InstrumentHttpClient, CannotInstrumentHttpMethod)

	data.WriteDiff()
}

func createDiffFile(path string) {
	f, err := os.Create(path)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
}

func CLISplash() {
	cmd := exec.Command("clear")
	cmd.Stdout = os.Stdout
	cmd.Run()

	fmt.Printf("New Relic Go Agent Pre Instrumentation Tool (Pre-Alpha)\n")
	fmt.Printf("-------------------------------------------------------\n")
	fmt.Printf("This tool will instrument your Go application with the New Relic Go Agent\n")
	fmt.Printf("And generate a diff file to show the changes made to your code\n")
	fmt.Printf("-------------------------------------------------------\n")
	fmt.Printf("\n")
	fmt.Printf("\n")

}
func CLIPrompts(packagePath, packageName, appName, agentVariableName, diffFileLocation string) {

	// Prompt user to enter the path to the package they want to instrument
	fmt.Printf("Enter the path to the package you want to instrument (default if left blank: '%s'):", defaultPackagePath)
	fmt.Scanln(&packagePath)
	if packagePath == "" {
		packagePath = defaultPackagePath
	}

	// Prompt user to enter the package name
	fmt.Printf("Enter the package name (default: '%s'):", defaultPackageName)
	fmt.Scanln(&packageName)

	if packageName == "" {
		packageName = defaultPackageName
	}

	// Prompt user to enter the application name
	fmt.Printf("Enter the application name (default: '%s'):", defaultAppName)
	fmt.Scanln(&appName)

	if appName == "" {
		appName = defaultAppName
	}

	// Prompt user to enter the agent variable name
	fmt.Printf("Enter the agent variable name (default: '%s'):", defaultAgentVariableName)
	fmt.Scanln(&agentVariableName)

	if agentVariableName == "" {
		agentVariableName = defaultAgentVariableName
	}

	// Prompt user to enter the diff file output location
	fmt.Printf("Enter the diff file output location (default: '%s/'):", diffFileLocation)
	fmt.Scanln(&diffFileLocation)

	if diffFileLocation == "" {
		// Set default diff file output location
		wd, _ := os.Getwd()
		diffFileLocation = wd

	}
	diffFile := fmt.Sprintf("%s/%s.diff", diffFileLocation, path.Base(packagePath))

	createDiffFile(diffFile)

}

func main() {
	// check if ran with -default flag
	isDefault := false
	for _, arg := range os.Args {
		if arg == "--default" {
			isDefault = true
		}
	}

	var packagePath, packageName, appName, agentVariableName, diffFileLocation string
	// Set default diff file output location
	wd, _ := os.Getwd()
	diffFileLocation = wd

	CLISplash()
	if !isDefault {
		CLIPrompts(packagePath, packageName, appName, agentVariableName, diffFileLocation)
	}

	diffFile := fmt.Sprintf("%s/%s.diff", diffFileLocation, path.Base(packagePath))
	createDiffFile(diffFile)

	//GoGetAgent(packagePath)

	loadMode := packages.LoadSyntax
	pkgs, err := decorator.Load(&packages.Config{Dir: packagePath, Mode: loadMode}, packageName)
	if err != nil {
		log.Fatal(err)
	}

	for _, pkg := range pkgs {
		InstrumentPackage(pkg, appName, agentVariableName, diffFile)
	}
}
