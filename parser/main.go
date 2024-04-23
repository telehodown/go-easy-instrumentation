package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"

	"golang.org/x/tools/go/ast/astutil"

	godiffpatch "github.com/sourcegraph/go-diff-patch"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}

type InstrumentationData struct {
	Fset              *token.FileSet
	AstFile           *ast.File
	AppName           string
	AgentVariableName string
}

type InstrumentationFunc func(n ast.Node, data *InstrumentationData) string

func preInstrumentation(data *InstrumentationData, instrumentationFunctions ...InstrumentationFunc) []string {
	downstreamFuncs := []string{}

	for i, d := range data.AstFile.Decls {
		newNode := astutil.Apply(d, nil, func(c *astutil.Cursor) bool {
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

		if n, ok := newNode.(*ast.FuncDecl); ok {
			data.AstFile.Decls[i] = n
		}
	}

	return downstreamFuncs
}

func mainInstrumentationLoop(data *InstrumentationData, instrumentationFunctions ...InstrumentationFunc) {
	for i, d := range data.AstFile.Decls {
		if fn, isFn := d.(*ast.FuncDecl); isFn {
			modifiedFunc := astutil.Apply(fn, nil, func(c *astutil.Cursor) bool {
				n := c.Node()
				for _, instFunc := range instrumentationFunctions {
					instFunc(n, data)
				}
				return true
			})
			if modifiedFunc != nil {
				data.AstFile.Decls[i] = modifiedFunc.(*ast.FuncDecl)
			}
		}
	}
}

func InstrumentFile(fileName, appName, agentVariableName string) {
	file, err := os.ReadFile(fileName)
	must(err)

	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, fileName, file, parser.ParseComments)
	must(err)

	data := InstrumentationData{
		Fset:              fset,
		AstFile:           astFile,
		AgentVariableName: agentVariableName,
		AppName:           appName,
	}

	// Pre Instrumentation Steps
	// 	- import the agent
	//	- initialize the agent
	//	- shutdown the agent
	downstreamFuncs := preInstrumentation(&data, InjectAgent, GetHandleFuncs)
	fmt.Printf("Downstream funcs: %+v\n", downstreamFuncs)

	// Main Instrumentation Loop
	//	- any instrumentation that consumes the agent
	mainInstrumentationLoop(&data, InstrumentHandleFunc)

	modifiedFile := bytes.NewBuffer([]byte{})
	printer.Fprint(modifiedFile, fset, astFile)

	patch := godiffpatch.GeneratePatch("../demo-app/main.go", string(file), modifiedFile.String())
	fmt.Println(patch)
}

func main() {
	fileName := "../demo-app/main.go"
	appName := "AST Example"
	agentVariableName := "NewRelicAgent"
	InstrumentFile(fileName, appName, agentVariableName)
}
