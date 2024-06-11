package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

	"github.com/dave/dst"
	"github.com/dave/dst/decorator"
	"github.com/dave/dst/dstutil"
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
		Decs: dst.IfStmtDecorations{
			NodeDecs: dst.NodeDecs{
				After: dst.EmptyLine,
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
		Decs: dst.ExprStmtDecorations{
			NodeDecs: dst.NodeDecs{
				Before: dst.EmptyLine,
			},
		},
	}
}

func startTransaction(appVariableName, transactionVariableName, transactionName string) *dst.AssignStmt {
	return &dst.AssignStmt{
		Lhs: []dst.Expr{dst.NewIdent(transactionVariableName)},
		Rhs: []dst.Expr{
			&dst.CallExpr{
				Args: []dst.Expr{
					&dst.BasicLit{
						Kind:  token.STRING,
						Value: fmt.Sprintf(`"%s"`, transactionName),
					},
				},
				Fun: &dst.SelectorExpr{
					X:   dst.NewIdent(appVariableName),
					Sel: dst.NewIdent("StartTransaction"),
				},
			},
		},
		Tok: token.DEFINE,
	}
}

func endTransaction(transactionVariableName string) *dst.ExprStmt {
	return &dst.ExprStmt{
		X: &dst.CallExpr{
			Fun: &dst.SelectorExpr{
				X:   dst.NewIdent(transactionVariableName),
				Sel: dst.NewIdent("End"),
			},
		},
	}
}

func addTxnToArguments(decl *dst.FuncDecl, txnVarName string) {
	decl.Type.Params.List = append(decl.Type.Params.List, &dst.Field{
		Names: []*dst.Ident{dst.NewIdent(txnVarName)},
		Type: &dst.StarExpr{
			X: &dst.SelectorExpr{
				X:   dst.NewIdent("newrelic"),
				Sel: dst.NewIdent("Transaction"),
			},
		},
	})
}

// InstrumentMain looks for the main method of a program, and uses this as an instrumentation initialization and injection point
func InstrumentMain(n dst.Node, data *InstrumentationManager, c *dstutil.Cursor) {
	if decl, ok := n.(*dst.FuncDecl); ok {
		// only inject go agent into the main.main function
		if decl.Name.Name == "main" {
			agentDecl := createAgentAST(data.appName, data.agentVariableName)
			decl.Body.List = append(agentDecl, decl.Body.List...)
			decl.Body.List = append(decl.Body.List, shutdownAgent(data.agentVariableName))

			newMain := dstutil.Apply(decl, func(c *dstutil.Cursor) bool {
				node := c.Node()
				switch v := node.(type) {
				case *dst.ExprStmt:
					fnName, call := data.GetPackageFunctionInvocation(v)
					if data.ShouldInstrumentFunction(fnName) {
						txnVarName := data.GenerateTransactionVariableName("")
						c.InsertBefore(startTransaction(data.agentVariableName, txnVarName, fnName))
						decl := data.GetDeclaration(fnName)
						_, wasModified := TraceFunction(data, decl, txnVarName)
						if wasModified {
							// add transaction to declaration and invocation arguments
							call.Args = append(call.Args, dst.NewIdent(txnVarName))
							addTxnToArguments(decl, txnVarName)
						}
						c.InsertAfter(endTransaction(txnVarName))

					}
					WrapHandleFunc(n, data, c)
				}

				return true
			}, nil)
			// this will skip the tracing of this function in the outer tree walking algorithm
			c.Replace(newMain)
		}
	}
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

func txnNoticeError(errVariableName, txnName string, nodeDecs *dst.NodeDecs) *dst.ExprStmt {
	// copy all decs below the current statement into this statement
	decs := dst.ExprStmtDecorations{
		NodeDecs: dst.NodeDecs{
			After: nodeDecs.After,
			End:   nodeDecs.End,
		},
	}

	// remove coppied decs from above node
	nodeDecs.After = dst.None
	nodeDecs.End.Clear()

	return &dst.ExprStmt{
		X: &dst.CallExpr{
			Fun: &dst.SelectorExpr{
				X: &dst.Ident{
					Name: txnName,
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
		Decs: decs,
	}
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

// NoticeError will check for the presence of an error.Error variable in the body at the index in bodyIndex.
// If it finds that an error is returned, it will add a line after the assignment statement to capture an error
// with a newrelic transaction. All transactions are assumed to be named "txn"
func NoticeError(data *InstrumentationManager, stmt dst.Stmt, c *dstutil.Cursor, txnName string) bool {
	switch nodeVal := stmt.(type) {
	case *dst.AssignStmt:
		errVar := findErrorVariable(nodeVal, data.pkg)
		if errVar != "" {
			c.InsertAfter(txnNoticeError(errVar, txnName, nodeVal.Decorations()))
			return true
		}
	}
	return false
}

// dfs search for async func block and injection of telemetry
func traceAsyncFunc(stmt *dst.GoStmt) {
	// Go function literal
	if fun, ok := stmt.Call.Fun.(*dst.FuncLit); ok {
		// Add threaded txn to function arguments and parameters
		fun.Type.Params.List = append(fun.Type.Params.List, txnAsParameter())
		stmt.Call.Args = append(stmt.Call.Args, txnNewGoroutine())

		// create async segment
		fun.Body.List = append([]dst.Stmt{deferAsyncSegment()}, fun.Body.List...)
	}
}

type TracingFunction func(data *InstrumentationManager, stmt dst.Stmt, c *dstutil.Cursor, tracingName string) bool

var tracingFuncs = []TracingFunction{ExternalHttpCall, InstrumentHandlerDeclaration, NoticeError}

// TraceFunction adds tracing to a function. This includes error capture, and passing agent metadata to relevant functions and services.
func TraceFunction(data *InstrumentationManager, fn *dst.FuncDecl, txnVarName string) (*dst.FuncDecl, bool) {
	wasChanged := false
	outputNode := dstutil.Apply(fn, nil, func(c *dstutil.Cursor) bool {
		n := c.Node()
		switch v := n.(type) {
		case *dst.GoStmt:
			traceAsyncFunc(v)
		case dst.Stmt:
			fnName, call := data.GetPackageFunctionInvocation(v)
			if data.ShouldInstrumentFunction(fnName) {
				decl := data.GetDeclaration(fnName)
				modifiedDecl, wasModified := TraceFunction(data, decl, txnVarName)
				if wasModified {
					wasChanged = true
					data.TraceFunctionDeclaration(modifiedDecl)
					call.Args = append(call.Args, dst.NewIdent(txnVarName))
					addTxnToArguments(decl, txnVarName)
				}
			}
			for _, stmtFunc := range tracingFuncs {
				ok := stmtFunc(data, v, c, txnVarName)
				if ok {
					wasChanged = true
				}
			}
		}
		return true
	})

	// update the stored declaration, marking it as traced
	decl := outputNode.(*dst.FuncDecl)
	data.TraceFunctionDeclaration(decl)
	return decl, wasChanged
}
