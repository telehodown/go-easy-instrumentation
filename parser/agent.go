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
	newappArgs := []dst.Expr{
		&dst.CallExpr{
			Fun: &dst.Ident{
				Path: newrelicAgentImport,
				Name: "ConfigFromEnvironment",
			},
		},
	}
	if AppName != "" {
		AppName = "\"" + AppName + "\""
		newappArgs = append([]dst.Expr{&dst.CallExpr{
			Fun: &dst.Ident{
				Path: newrelicAgentImport,
				Name: "ConfigAppName",
			},
			Args: []dst.Expr{
				&dst.BasicLit{
					Kind:  token.STRING,
					Value: AppName,
				},
			},
		}}, newappArgs...)
	}

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
				Fun: &dst.Ident{
					Name: "NewApplication",
					Path: newrelicAgentImport,
				},
				Args: newappArgs,
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

func startTransaction(appVariableName, transactionVariableName, transactionName string, overwriteVariable bool) *dst.AssignStmt {
	tok := token.DEFINE
	if overwriteVariable {
		tok = token.ASSIGN
	}
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
		Tok: tok,
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

// InstrumentMain looks for the main method of a program, and uses this as an instrumentation initialization and injection point
func InstrumentMain(mainFunctionNode dst.Node, data *InstrumentationManager, c *dstutil.Cursor) {
	txnStarted := false
	if decl, ok := mainFunctionNode.(*dst.FuncDecl); ok {
		// only inject go agent into the main.main function
		if decl.Name.Name == "main" {
			agentDecl := createAgentAST(data.appName, data.agentVariableName)
			decl.Body.List = append(agentDecl, decl.Body.List...)
			decl.Body.List = append(decl.Body.List, shutdownAgent(data.agentVariableName))

			// add go-agent/v3/newrelic to imports
			data.AddImport(newrelicAgentImport)

			newMain := dstutil.Apply(decl, func(c *dstutil.Cursor) bool {
				node := c.Node()
				switch v := node.(type) {
				case *dst.ExprStmt:
					fnName, call := data.GetPackageFunctionInvocation(v)
					// check if the called function has been instrumented already, if not, instrument it.
					if data.ShouldInstrumentFunction(fnName) {
						decl := data.GetDeclaration(fnName)
						_, wasModified := TraceFunction(data, decl, defaultTxnName)
						if wasModified {
							// add transaction to declaration arguments
							data.AddTxnArgumentToFunctionDecl(decl, defaultTxnName, fnName)
							data.AddImport(newrelicAgentImport)
						}
					}
					// pass the called function a transaction if needed
					if data.RequiresTransactionArgument(fnName) {
						txnVarName := defaultTxnName
						c.InsertBefore(startTransaction(data.agentVariableName, txnVarName, fnName, txnStarted))
						c.InsertAfter(endTransaction(txnVarName))
						call.Args = append(call.Args, dst.NewIdent(defaultTxnName))
						txnStarted = true
					}
					WrapHandleFunc(v.X, data, c)
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

func deferSegment(segmentName, txnVarName string) *dst.DeferStmt {
	return &dst.DeferStmt{
		Call: &dst.CallExpr{
			Fun: &dst.SelectorExpr{
				X: &dst.CallExpr{
					Fun: &dst.SelectorExpr{
						X: dst.NewIdent(txnVarName),
						Sel: &dst.Ident{
							Name: "StartSegment",
						},
					},
					Args: []dst.Expr{
						&dst.BasicLit{
							Kind:  token.STRING,
							Value: fmt.Sprintf(`"%s"`, segmentName),
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
	astCall, ok := pkg.Decorator.Ast.Nodes[v]
	if ok {
		ty := pkg.TypesInfo.TypeOf(astCall.(*ast.CallExpr))
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

type TracingFunction func(data *InstrumentationManager, stmt dst.Stmt, c *dstutil.Cursor, tracingName string) bool

var tracingFuncs = []TracingFunction{ExternalHttpCall, WrapNestedHandleFunction, NoticeError}

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

// TraceFunction adds tracing to a function. This includes error capture, and passing agent metadata to relevant functions and services.
// Traces all called functions inside the current package as well.
// This function returns a FuncDecl object pointer that contains the potentially modified version of the FuncDecl object, fn, passed. If
// the bool field is true, then the function was modified, and requires a transaction most likely.
func TraceFunction(data *InstrumentationManager, fn *dst.FuncDecl, txnVarName string) (*dst.FuncDecl, bool) {
	TopLevelFunctionChanged := false
	outputNode := dstutil.Apply(fn, nil, func(c *dstutil.Cursor) bool {
		n := c.Node()
		switch v := n.(type) {
		case *dst.GoStmt:
			switch fun := v.Call.Fun.(type) {
			case *dst.FuncLit:
				// Add threaded txn to function arguments and parameters
				fun.Type.Params.List = append(fun.Type.Params.List, txnAsParameter())
				v.Call.Args = append(v.Call.Args, txnNewGoroutine())
				// add go-agent/v3/newrelic to imports
				data.AddImport(newrelicAgentImport)

				// create async segment
				fun.Body.List = append([]dst.Stmt{deferSegment("async literal", txnVarName)}, fun.Body.List...)
				c.Replace(v)
				TopLevelFunctionChanged = true
			default:
				fnName, call := data.GetPackageFunctionInvocation(v.Call)
				if data.ShouldInstrumentFunction(fnName) {
					decl := data.GetDeclaration(fnName)
					TraceFunction(data, decl, txnVarName)
					data.AddTxnArgumentToFunctionDecl(decl, txnVarName, fnName)
					data.AddImport(newrelicAgentImport)
					decl.Body.List = append([]dst.Stmt{deferSegment(fmt.Sprintf("async %s", fnName), txnVarName)}, decl.Body.List...)
				}
				if data.RequiresTransactionArgument(fnName) {
					call.Args = append(call.Args, dst.NewIdent(txnVarName))
					c.Replace(v)
					TopLevelFunctionChanged = true
				}
			}
		case dst.Stmt:
			fnName, call := data.GetPackageFunctionInvocation(v)
			if data.ShouldInstrumentFunction(fnName) {
				decl := data.GetDeclaration(fnName)
				_, wasModified := TraceFunction(data, decl, txnVarName)
				if wasModified {
					data.AddTxnArgumentToFunctionDecl(decl, txnVarName, fnName)
					data.AddImport(newrelicAgentImport)
					decl.Body.List = append([]dst.Stmt{deferSegment(fnName, txnVarName)}, decl.Body.List...)
				}
			}
			if data.RequiresTransactionArgument(fnName) {
				call.Args = append(call.Args, dst.NewIdent(txnVarName))
				TopLevelFunctionChanged = true
			}
			for _, stmtFunc := range tracingFuncs {
				ok := stmtFunc(data, v, c, txnVarName)
				if ok {
					TopLevelFunctionChanged = true
				}
			}
		}
		return true
	})

	// update the stored declaration, marking it as traced
	decl := outputNode.(*dst.FuncDecl)
	data.UpdateFunctionDeclaration(decl)
	return decl, TopLevelFunctionChanged
}
