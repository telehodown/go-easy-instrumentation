package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"

	"github.com/dave/dst"
	"github.com/dave/dst/decorator"
	"github.com/dave/dst/decorator/resolver/gopackages"
	"github.com/dave/dst/dstutil"
	"golang.org/x/tools/go/packages"
)

type sourceKind uint8

const (
	txnInCtx        sourceKind = 1
	txnArgument     sourceKind = 2
	appArgument     sourceKind = 3
	httpRespContext sourceKind = 4
)

type tracingData struct {
	traced bool
	kind   sourceKind
}

type InstrumentationData struct {
	pkg               *decorator.Package
	appName           string
	agentVariableName string
	tracedFuncs       map[string]tracingData
	imports           map[string][]string
}

func NewInstrumentationData(pkg *decorator.Package, appName, agentVariableName string) *InstrumentationData {
	return &InstrumentationData{
		pkg:               pkg,
		appName:           appName,
		agentVariableName: agentVariableName,
		tracedFuncs:       map[string]tracingData{},
		imports:           map[string][]string{},
	}
}

func (d *InstrumentationData) AddImport(importName, fileName string) {

}

// AddTrace adds data to the cache to keep track of what top level functions may need additional downstream tracing
func (d *InstrumentationData) AddTrace(functionName string, kind sourceKind) {
	_, ok := d.tracedFuncs[functionName]
	if !ok {
		d.tracedFuncs[functionName] = tracingData{
			traced: false,
			kind:   kind,
		}
	}
}

// MarkTracingComplete identifies a function as being fully traced, preventing duplication of work.
func (d *InstrumentationData) MarkTracingComplete(functionName string) {
	data := d.tracedFuncs[functionName]
	if !data.traced {
		data.traced = true
		d.tracedFuncs[functionName] = data
	}
}

// IsTraced returns true if a function has already had tracing added to it.
func (d *InstrumentationData) IsTraced(functionName string) bool {
	return d.tracedFuncs[functionName].traced
}

type InstrumentationFunc func(n dst.Node, data *InstrumentationData)

func preInstrumentation(data *InstrumentationData, instrumentationFunctions ...InstrumentationFunc) {
	for fileIndx, file := range data.pkg.Syntax {
		for declIndex, d := range file.Decls {
			if fn, isFn := d.(*dst.FuncDecl); isFn {
				modifiedFunc := dstutil.Apply(fn, nil, func(c *dstutil.Cursor) bool {
					n := c.Node()
					if n != nil {
						for _, instFunc := range instrumentationFunctions {
							instFunc(n, data)
						}
					}
					return true
				})
				if modifiedFunc != nil {
					data.pkg.Syntax[fileIndx].Decls[declIndex] = modifiedFunc.(*dst.FuncDecl)
				}
			}
		}
	}
}

func downstreamInstrumentation(data *InstrumentationData, instrumentationFunctions ...InstrumentationFunc) {
	for fileIndex, file := range data.pkg.Syntax {
		for declIndex, d := range file.Decls {
			if fn, isFn := d.(*dst.FuncDecl); isFn {
				modifiedFunc := dstutil.Apply(fn, nil, func(c *dstutil.Cursor) bool {
					n := c.Node()
					if n != nil {
						for _, instFunc := range instrumentationFunctions {
							instFunc(n, data)
						}
					}
					return true
				})
				if modifiedFunc != nil {
					data.pkg.Syntax[fileIndex].Decls[declIndex] = modifiedFunc.(*dst.FuncDecl)
				}
			}
		}
	}
}

func InstrumentPackage(pkg *decorator.Package, pkgPath, appName, agentVariableName string) {
	data := NewInstrumentationData(pkg, appName, agentVariableName)

	// Pre Instrumentation Steps
	// 	- import the agent
	//	- initialize the agent
	//	- shutdown the agent
	preInstrumentation(data, InjectAgent, InstrumentHandleFunc, InstrumentHandler)

	// Main Instrumentation Loop
	//	- any instrumentation that consumes the agent
	downstreamInstrumentation(data)

	r := decorator.NewRestorerWithImports(pkgPath, gopackages.New(pkg.Dir))
	for _, file := range pkg.Syntax {
		modifiedFile := bytes.NewBuffer([]byte{})
		if err := r.Fprint(modifiedFile, file); err != nil {
			panic(err)
		}

		fmt.Println(modifiedFile.String())

		//		patch := godiffpatch.GeneratePatch(file.Name.String(), File, modifiedFile.String())
		//		fmt.Println(patch)

	}

}

func GoGetAgent(packagePath string) {
	wd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	if packagePath != wd {
		os.Chdir(packagePath)
	}

	cmd := exec.Command("go", "get", "github.com/newrelic/go-agent/v3")
	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}
}

func main() {
	packagePath := "/Users/emiliogarcia/Dev/go-agent-pre-instrumentation/demo-app"
	packageName := "."
	appName := "AST Example"
	agentVariableName := "NewRelicAgent"

	GoGetAgent(packagePath)

	loadMode := packages.LoadSyntax
	pkgs, err := decorator.Load(&packages.Config{Dir: packagePath, Mode: loadMode}, packageName)
	if err != nil {
		log.Fatal(err)
	}

	for _, pkg := range pkgs {
		InstrumentPackage(pkg, packagePath, appName, agentVariableName)
	}
}
