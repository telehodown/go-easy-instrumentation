package main

import (
	"go/ast"
	"go/token"
	"go/types"

	"github.com/dave/dst"
	"github.com/dave/dst/decorator"
)

const (
// agentImport = "github.com/newrelic/go-agent/v3"
)

func panicOnError() *dst.IfStmt {
	return &dst.IfStmt{
		Cond: &dst.BinaryExpr{
			X: &dst.Ident{
				Name: "err",
			},
			Op: token.NEQ,
			Y: &dst.Ident{
				Name: "nil",
			},
		},
		Body: &dst.BlockStmt{
			List: []dst.Stmt{
				&dst.ExprStmt{
					X: &dst.CallExpr{
						Fun: &dst.Ident{
							Name: "panic",
						},
						Args: []dst.Expr{
							&dst.Ident{
								Name: "err",
							},
						},
					},
				},
			},
		},
	}
}

func createAgentAST(AppName, AgentVariableName string) []dst.Stmt {
	AppName = "\"" + AppName + "\""

	agentInit := &dst.AssignStmt{
		Lhs: []dst.Expr{
			&dst.Ident{
				Name: AgentVariableName,
			},
			&dst.Ident{
				Name: "err",
			},
		},
		Tok: token.DEFINE,
		Rhs: []dst.Expr{
			&dst.CallExpr{
				Fun: &dst.SelectorExpr{
					X: &dst.Ident{
						Name: "newrelic",
					},
					Sel: &dst.Ident{
						Name: "NewApplication",
					},
				},
				Args: []dst.Expr{
					&dst.CallExpr{
						Fun: &dst.SelectorExpr{
							X: &dst.Ident{
								Name: "newrelic",
							},
							Sel: &dst.Ident{
								Name: "ConfigAppName",
							},
						},
						Args: []dst.Expr{
							&dst.BasicLit{
								Kind:  token.STRING,
								Value: AppName,
							},
						},
					},
					&dst.CallExpr{
						Fun: &dst.SelectorExpr{
							X: &dst.Ident{
								Name: "newrelic",
							},
							Sel: &dst.Ident{
								Name: "ConfigFromEnvironment",
							},
						},
					},
				},
			},
		},
	}

	return []dst.Stmt{agentInit, panicOnError()}
}

func shutdownAgent(AgentVariableName string) *dst.ExprStmt {
	return &dst.ExprStmt{
		X: &dst.CallExpr{
			Fun: &dst.SelectorExpr{
				X: &dst.Ident{
					Name: AgentVariableName,
				},
				Sel: &dst.Ident{
					Name: "Shutdown",
				},
			},
			Args: []dst.Expr{
				&dst.BinaryExpr{
					X: &dst.BasicLit{
						Kind:  token.INT,
						Value: "5",
					},
					Op: token.MUL,
					Y: &dst.SelectorExpr{
						X: &dst.Ident{
							Name: "time",
						},
						Sel: &dst.Ident{
							Name: "Second",
						},
					},
				},
			},
		},
	}
}

func txnFromCtx() *dst.AssignStmt {
	return &dst.AssignStmt{
		Lhs: []dst.Expr{
			&dst.Ident{
				Name: "txn",
			},
		},
		Tok: token.DEFINE,
		Rhs: []dst.Expr{
			&dst.CallExpr{
				Fun: &dst.SelectorExpr{
					X: &dst.Ident{
						Name: "newrelic",
					},
					Sel: &dst.Ident{
						Name: "FromContext",
					},
				},
				Args: []dst.Expr{
					&dst.CallExpr{
						Fun: &dst.SelectorExpr{
							X: &dst.Ident{
								Name: "r",
							},
							Sel: &dst.Ident{
								Name: "Context",
							},
						},
					},
				},
			},
		},
	}
}

/*
func containsAgentImport(imports []*dst.ImportSpec) bool {
	for _, imp := range imports {
		if imp.Path.Value == agentImport {
			return true
		}
	}
	return false
}

func ImportAgent(fileset *token.FileSet, file *dst.File) string {
	if !containsAgentImport(file.Imports) {
		dstutil.AddImport(fileset, file, agentImport)
		return ""
	}
	return ""
}
*/

func InjectAgent(n dst.Node, data *InstrumentationData) string {
	if decl, ok := n.(*dst.FuncDecl); ok {
		// only inject go agent into the main.main function
		if data.pkg.Name == "main" && decl.Name.Name == "main" {
			agentDecl := createAgentAST(data.appName, data.agentVariableName)
			decl.Body.List = append(agentDecl, decl.Body.List...)
			decl.Body.List = append(decl.Body.List, shutdownAgent(data.agentVariableName))
			return ""
		}
	}
	return ""
}

func txnAsParameter() *dst.Field {
	return &dst.Field{
		Names: []*dst.Ident{
			{
				Name: "txn",
			},
		},
		Type: &dst.StarExpr{
			X: &dst.SelectorExpr{
				X: &dst.Ident{
					Name: "newrelic",
				},
				Sel: &dst.Ident{
					Name: "Transaction",
				},
			},
		},
	}
}

func deferAsyncSegment() *dst.DeferStmt {
	return &dst.DeferStmt{
		Call: &dst.CallExpr{
			Fun: &dst.SelectorExpr{
				X: &dst.CallExpr{
					Fun: &dst.SelectorExpr{
						X: &dst.Ident{
							Name: "txn",
						},
						Sel: &dst.Ident{
							Name: "StartSegment",
						},
					},
					Args: []dst.Expr{
						&dst.BasicLit{
							Kind:  token.STRING,
							Value: "\"async\"",
						},
					},
				},
				Sel: &dst.Ident{
					Name: "End",
				},
			},
		},
	}
}

func txnNewGoroutine() *dst.CallExpr {
	return &dst.CallExpr{
		Fun: &dst.SelectorExpr{
			X: &dst.Ident{
				Name: "txn",
			},
			Sel: &dst.Ident{
				Name: "NewGoroutine",
			},
		},
	}
}

// dfs search for async func block and injection of telemetry
func traceAsyncFunc(stmt *dst.GoStmt, tracedFuncs map[string]bool) {
	// Go function literal
	if fun, ok := stmt.Call.Fun.(*dst.FuncLit); ok {
		// Add threaded txn to function arguments and parameters
		fun.Type.Params.List = append(fun.Type.Params.List, txnAsParameter())
		stmt.Call.Args = append(stmt.Call.Args, txnNewGoroutine())

		// create async segment
		fun.Body.List = append([]dst.Stmt{deferAsyncSegment()}, fun.Body.List...)
	} else {
		if fun, ok := stmt.Call.Fun.(*dst.CallExpr); ok {
			fun.Args = append(fun.Args, txnNewGoroutine())
			if sel, ok := fun.Fun.(*dst.SelectorExpr); ok {
				tracedFuncs[sel.Sel.Name] = true
			}
		}
	}
}

func isNamedError(n *types.Named) bool {
	o := n.Obj()
	return o != nil && o.Pkg() == nil && o.Name() == "error"
}

func errorReturns(v *dst.CallExpr, pkg *decorator.Package) (int, bool) {
	astCall := pkg.Decorator.Ast.Nodes[v].(*ast.CallExpr)
	ty := pkg.TypesInfo.TypeOf(astCall)
	switch n := ty.(type) {
	case *types.Named:
		if isNamedError(n) {
			return 0, true
		}
	case *types.Tuple:
		for i := 0; i < n.Len(); i++ {
			t := n.At(i).Type()
			switch e := t.(type) {
			case *types.Named:
				if isNamedError(e) {
					return i, true
				}
			}
		}
	}
	return 0, false
}

func isNewRelicMethod(call *dst.CallExpr) bool {
	if sel, ok := call.Fun.(*dst.SelectorExpr); ok {
		if pkg, ok := sel.X.(*dst.Ident); ok {
			return pkg.Name == "newrelic"
		}
	}
	return false
}

func findErrorVariable(stmt *dst.AssignStmt, pkg *decorator.Package) string {
	if len(stmt.Rhs) == 1 {
		if call, ok := stmt.Rhs[0].(*dst.CallExpr); ok {
			if !isNewRelicMethod(call) {
				if errIndex, ok := errorReturns(call, pkg); ok {
					expr := stmt.Lhs[errIndex]
					if ident, ok := expr.(*dst.Ident); ok {
						return ident.Name
					}
				}
			}
		}
	}
	return ""
}

func txnNoticeError(errVariableName string) *dst.ExprStmt {
	return &dst.ExprStmt{
		X: &dst.CallExpr{
			Fun: &dst.SelectorExpr{
				X: &dst.Ident{
					Name: "txn",
				},
				Sel: &dst.Ident{
					Name: "NoticeError",
				},
			},
			Args: []dst.Expr{
				&dst.Ident{
					Name: errVariableName,
				},
			},
		},
	}
}

// NoticeError will check for the presence of an error.Error variable in the body at the index in bodyIndex.
// If it finds that an error is returned, it will add a line after the assignment statement to capture an error
// with a newrelic transaction. All transactions are assumed to be named "txn"
func NoticeError(stmt *dst.AssignStmt, pkg *decorator.Package, body []dst.Stmt, bodyIndex int) []dst.Stmt {
	errVar := findErrorVariable(stmt, pkg)
	if errVar != "" {
		newBody := []dst.Stmt{}
		newBody = append(newBody, body[:bodyIndex+1]...)
		newBody = append(newBody, txnNoticeError(errVar))
		newBody = append(newBody, body[bodyIndex+1:]...)
		return newBody
	}
	return body
}

func TraceFunction(data *InstrumentationData, body []dst.Stmt, txnName string) []dst.Stmt {
	instrumentedBody := body
	for i, stmt := range instrumentedBody {
		switch v := stmt.(type) {
		case *dst.GoStmt:
			traceAsyncFunc(v, data.tracedFuncs)
		case *dst.ForStmt:
			TraceFunction(data, v.Body.List, txnName)
		case *dst.AssignStmt:
			instrumentedBody = NoticeError(v, data.pkg, instrumentedBody, i)
		}
	}
	return instrumentedBody
}
