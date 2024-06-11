package main

import (
	"bytes"
	"go/ast"
	"go/types"
	"log"
	"os"
	"strconv"

	"github.com/dave/dst"
	"github.com/dave/dst/decorator"
	"github.com/dave/dst/decorator/resolver/gopackages"
	godiffpatch "github.com/sourcegraph/go-diff-patch"
)

const (
	defaultTxnName = "nrTxn"
)

// tracedFunction contains relevant information about a function within the current package, and
// its tracing status.
//
// Please access this object's data through methods rather than directly manipulating it.
type tracedFunction struct {
	traced bool
	body   *dst.FuncDecl
}

// InstrumentationManager maintains state relevant to tracing across all files and functions within a package.
type InstrumentationManager struct {
	diffFile          string
	appName           string
	agentVariableName string
	pkg               *decorator.Package
	tracedFuncs       map[string]*tracedFunction
	txnVariableNames  map[string]int
}

// NewInstrumentationManager initializes an InstrumentationManager cache for a given package.
func NewInstrumentationManager(pkg *decorator.Package, appName, agentVariableName, diffFile string) *InstrumentationManager {
	return &InstrumentationManager{
		diffFile:          diffFile,
		pkg:               pkg,
		appName:           appName,
		agentVariableName: agentVariableName,
		tracedFuncs:       map[string]*tracedFunction{},
		txnVariableNames:  map[string]int{},
	}
}

// GenerateTransactionVariableName ensures that no illegal naming occurs and generates a unique variable name
func (d *InstrumentationManager) GenerateTransactionVariableName(names ...string) string {
	variableName := defaultTxnName
	if len(names) > 0 {
		for i, name := range names {
			if i == len(names)-1 {
				variableName = variableName + name
			} else {
				variableName = variableName + name + "_"
			}
		}
	}

	count := d.txnVariableNames[variableName]
	if count > 0 {
		variableName = variableName + strconv.Itoa(count)
	}
	d.txnVariableNames[variableName] = count + 1
	return variableName
}

// TraceFunction creates a tracking object for a function declaration that can be used
// to find tracing locations, and the status of that tracing.
func (d *InstrumentationManager) TraceFunctionDeclaration(decl *dst.FuncDecl) {
	t, ok := d.tracedFuncs[decl.Name.Name]
	if ok {
		if decl == t.body {
			return
		}
		t.body = decl
		t.traced = true
	} else {
		d.tracedFuncs[decl.Name.Name] = &tracedFunction{
			body: decl,
		}
	}
}

func (d *InstrumentationManager) GetPackageFunctionInvocation(node dst.Node) (string, *dst.CallExpr) {
	fnName := ""
	var pkgCall *dst.CallExpr
	dst.Inspect(node, func(n dst.Node) bool {
		switch v := n.(type) {
		case *dst.CallExpr:
			call := v
			functionCallIdent, ok := call.Fun.(*dst.Ident)
			if ok {
				astNode := d.pkg.Decorator.Ast.Nodes[functionCallIdent]
				switch astNodeType := astNode.(type) {
				case *ast.SelectorExpr:
					pkgID := astNodeType.X.(*ast.Ident)
					callPackage := d.pkg.TypesInfo.Uses[pkgID]
					if callPackage.(*types.PkgName).Imported().Path() == d.pkg.PkgPath {
						fnName = astNodeType.Sel.Name
						pkgCall = call
						return false
					}
				case *ast.Ident:
					pkgID := astNodeType
					callPackage := d.pkg.TypesInfo.Uses[pkgID]
					if callPackage.Pkg().Path() == d.pkg.PkgPath {
						fnName = pkgID.Name
						pkgCall = call
						return false
					}
				}
			}
			return true
		}
		return true
	})

	return fnName, pkgCall
}

// MarkTracingComplete identifies a function as being fully traced, preventing duplication of work.
func (d *InstrumentationManager) MarkTracingCompleted(functionName string) {
	data := d.tracedFuncs[functionName]
	data.traced = true
}

// IsTracingComplete returns true if a function has all the tracing it needs added to it.
func (d *InstrumentationManager) ShouldInstrumentFunction(functionName string) bool {
	if functionName == "" {
		return false
	}
	v, ok := d.tracedFuncs[functionName]
	if ok {
		return !v.traced
	}

	return false
}

// GetDeclaration returns a pointer to the location in the DST tree where a function is declared and defined.
func (d *InstrumentationManager) GetDeclaration(functionName string) *dst.FuncDecl {
	v, ok := d.tracedFuncs[functionName]
	if ok {
		return v.body
	}
	return nil
}

// WriteDiff writes out the changes made to a file to the diff file for this package.
func (d *InstrumentationManager) WriteDiff() {
	r := decorator.NewRestorerWithImports(d.pkg.Dir, gopackages.New(d.pkg.Dir))
	for _, file := range d.pkg.Syntax {
		fName := d.pkg.Decorator.Filenames[file]
		originalFile, err := os.ReadFile(fName)
		if err != nil {
			log.Fatal(err)
		}

		modifiedFile := bytes.NewBuffer([]byte{})
		if err := r.Fprint(modifiedFile, file); err != nil {
			panic(err)
		}
		f, err := os.OpenFile(d.diffFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Println(err)
		}
		defer f.Close()
		patch := godiffpatch.GeneratePatch(fName[1:], string(originalFile), modifiedFile.String())
		if _, err := f.WriteString(patch); err != nil {
			log.Println(err)
		}
	}
}
