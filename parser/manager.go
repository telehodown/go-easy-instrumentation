package main

import (
	"bytes"
	"go/ast"
	"go/types"
	"log"
	"os"

	"github.com/dave/dst"
	"github.com/dave/dst/decorator"
	"github.com/dave/dst/decorator/resolver/gopackages"
	godiffpatch "github.com/sourcegraph/go-diff-patch"
)

// invocation contains a pointer to the node in the dst tree where a function
// call is invoked, as well as the name of the calling function that it was invoked within.
//
// The caller can be used to back track up the call chain, and its body can be searched in the
// traced functions data structure by name.
type invocation struct {
	call   *dst.CallExpr
	caller string
}

// tracedFunction contains relevant information about a function within the current package, and
// its tracing status.
//
// Please access this object's data through methods rather than directly manipulating it.
type tracedFunction struct {
	traced      bool
	body        *dst.FuncDecl
	invocations []*invocation
}

// InstrumentationManager maintains state relevant to tracing across all files and functions within a package.
type InstrumentationManager struct {
	diffFile          string
	appName           string
	agentVariableName string
	pkg               *decorator.Package
	tracedFuncs       map[string]*tracedFunction
}

// NewInstrumentationManager initializes an InstrumentationManager cache for a given package.
func NewInstrumentationManager(pkg *decorator.Package, appName, agentVariableName, diffFile string) *InstrumentationManager {
	return &InstrumentationManager{
		diffFile:          diffFile,
		pkg:               pkg,
		appName:           appName,
		agentVariableName: agentVariableName,
		tracedFuncs:       map[string]*tracedFunction{},
	}
}

// TraceFunction creates a tracking object for a function declaration that can be used
// to find tracing locations, and the status of that tracing.
func (d *InstrumentationManager) TraceFunctionDeclaration(decl *dst.FuncDecl) {
	t, ok := d.tracedFuncs[decl.Name.Name]
	if ok {
		t.body = decl
	} else {
		d.tracedFuncs[decl.Name.Name] = &tracedFunction{
			body: decl,
		}
	}
}

// TraceFunctionCall traces a function call made with a function defined in the package being analyzed.
// It looks for a function call within the body of a statement, and if that function call is part of
// the current package, it adds its tracing information to the InstrumentationManager object.
func (d *InstrumentationManager) TraceFuncionCall(stmt dst.Stmt, caller string) {
	dst.Inspect(stmt, func(n dst.Node) bool {
		var fnName string
		switch call := n.(type) {
		case *dst.CallExpr:
			functionCallIdent, ok := call.Fun.(*dst.Ident)
			if ok {
				astNode := d.pkg.Decorator.Ast.Nodes[functionCallIdent]
				switch astNodeType := astNode.(type) {
				case *ast.SelectorExpr:
					pkgID := astNodeType.X.(*ast.Ident)
					callPackage := d.pkg.TypesInfo.Uses[pkgID]
					if callPackage.(*types.PkgName).Imported().Path() == d.pkg.PkgPath {
						fnName = astNodeType.Sel.Name
					}
				case *ast.Ident:
					pkgID := astNodeType
					callPackage := d.pkg.TypesInfo.Uses[pkgID]
					if callPackage.Pkg().Path() == d.pkg.PkgPath {
						fnName = pkgID.Name
					}
				}
			}

			if fnName != "" {
				t, ok := d.tracedFuncs[fnName]
				if ok {
					t.invocations = append(t.invocations, &invocation{call, caller})
				}

				// stop traversing, we found what we are looking for
				return false
			}
		}

		return true
	})
}

// MarkTracingComplete identifies a function as being fully traced, preventing duplication of work.
func (d *InstrumentationManager) MarkTracingCompleted(functionName string) {
	data := d.tracedFuncs[functionName]
	data.traced = true
}

// IsTracingComplete returns true if a function has all the tracing it needs added to it.
func (d *InstrumentationManager) IsTracingComplete(functionName string) bool {
	v, ok := d.tracedFuncs[functionName]
	if ok {
		return v.traced
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

// GetInvocations returns all the locations in the current package where a function call is made that invokes
// a function that is declared in the current package.
func (d *InstrumentationManager) GetInvocations(functionName string) []*invocation {
	v, ok := d.tracedFuncs[functionName]
	if ok {
		return v.invocations
	}
	return nil
}

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
