package main

import (
	"bytes"
	"fmt"

	"github.com/dave/dst"
	"github.com/dave/dst/decorator"
	"github.com/dave/dst/decorator/resolver/gopackages"
	"github.com/dave/dst/dstutil"
	"golang.org/x/tools/go/packages"
)

type InstrumentationData struct {
	pkg               *decorator.Package
	appName           string
	agentVariableName string
	tracedFuncs       map[string]bool
}

type InstrumentationFunc func(n dst.Node, data *InstrumentationData) string

func preInstrumentation(data *InstrumentationData, instrumentationFunctions ...InstrumentationFunc) []string {
	downstreamFuncs := []string{}

	for _, file := range data.pkg.Syntax {
		for i, d := range file.Decls {
			if fn, isFn := d.(*dst.FuncDecl); isFn {
				newNode := dstutil.Apply(fn, nil, func(c *dstutil.Cursor) bool {
					n := c.Node()
					if n != nil {
						for _, instFunc := range instrumentationFunctions {
							downstream := instFunc(n, data)
							if downstream != "" {
								downstreamFuncs = append(downstreamFuncs, downstream)
							}
						}
					}
					return true
				})

				if n, ok := newNode.(*dst.FuncDecl); ok {
					file.Decls[i] = n
				}
			}
		}
	}

	return downstreamFuncs
}

func downstreamInstrumentation(data *InstrumentationData, instrumentationFunctions ...InstrumentationFunc) {
	for _, file := range data.pkg.Syntax {
		for i, d := range file.Decls {
			if fn, isFn := d.(*dst.FuncDecl); isFn {
				modifiedFunc := dstutil.Apply(fn, nil, func(c *dstutil.Cursor) bool {
					n := c.Node()
					for _, instFunc := range instrumentationFunctions {
						instFunc(n, data)
					}
					return true
				})
				if modifiedFunc != nil {
					file.Decls[i] = modifiedFunc.(*dst.FuncDecl)
				}
			}
		}
	}
}

func InstrumentPackage(pkg *decorator.Package, pkgPath, appName, agentVariableName string) {
	data := InstrumentationData{
		pkg:               pkg,
		appName:           appName,
		agentVariableName: agentVariableName,
	}

	// Pre Instrumentation Steps
	// 	- import the agent
	//	- initialize the agent
	//	- shutdown the agent
	preInstrumentation(&data, InjectAgent, InstrumentHandleFunc)

	// Main Instrumentation Loop
	//	- any instrumentation that consumes the agent
	downstreamInstrumentation(&data)

	r := decorator.NewRestorerWithImports(pkgPath, gopackages.New(pkg.Dir))
	for _, file := range pkg.Syntax {
		modifiedFile := bytes.NewBuffer([]byte{})
		if err := r.Fprint(modifiedFile, file); err != nil {
			panic(err)
		}

		fmt.Println(modifiedFile.String())

		//patch := godiffpatch.GeneratePatch(file.Name.String(), File, modifiedFile.String())
		//fmt.Println(patch)

	}

}

func main() {
	packagePath := "/Users/emiliogarcia/Dev/go-agent-pre-instrumentation/demo-app"
	packageName := "."
	appName := "AST Example"
	agentVariableName := "NewRelicAgent"
	pkgs, err := decorator.Load(&packages.Config{Dir: packagePath, Mode: packages.LoadSyntax}, packageName)
	if err != nil {
		panic(err)
	}

	for _, pkg := range pkgs {
		InstrumentPackage(pkg, packagePath, appName, agentVariableName)
	}
}
