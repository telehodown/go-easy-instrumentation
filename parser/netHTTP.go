package main

import (
	"go/ast"
	"go/token"

	"github.com/dave/dst"
)

func isNetHTTPHandleFunc(n *dst.CallExpr) bool {
	ident, ok := n.Fun.(*dst.Ident)
	if ok {
		return ident.Path == "net/http" && ident.Name == "HandleFunc"
	}

	return false
}

func InstrumentHandleFunc(n dst.Node, data *InstrumentationData) {
	callExpr, ok := n.(*dst.CallExpr)
	if ok {
		if isNetHTTPHandleFunc(callExpr) && len(callExpr.Args) == 2 {
			// Capture name of handle funcs for deeper instrumentation
			handleFunc, ok := callExpr.Args[1].(*dst.Ident)
			if ok {
				data.AddTrace(handleFunc.Name, httpRespContext)
			}

			// Instrument handle funcs
			oldArgs := callExpr.Args
			callExpr.Args = []dst.Expr{
				&dst.CallExpr{
					Fun: &dst.SelectorExpr{
						X: &dst.Ident{
							Name: "newrelic",
						},
						Sel: &dst.Ident{
							Name: "WrapHandleFunc",
						},
					},
					Args: []dst.Expr{
						&dst.Ident{
							Name: data.agentVariableName,
						},
						oldArgs[0],
						oldArgs[1],
					},
				},
			}
		}
	}
}

func txnFromCtx(txnVariable string) *dst.AssignStmt {
	return &dst.AssignStmt{
		Decs: dst.AssignStmtDecorations{
			NodeDecs: dst.NodeDecs{
				After: dst.EmptyLine,
			},
		},
		Lhs: []dst.Expr{
			&dst.Ident{
				Name: txnVariable,
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

func isHttpHandler(params []*dst.Field, data *InstrumentationData) bool {
	if len(params) == 2 {
		var rw, req bool
		for _, param := range params {
			ident, ok := param.Type.(*dst.Ident)
			star, okStar := param.Type.(*dst.StarExpr)
			if ok {
				astNode := data.pkg.Decorator.Ast.Nodes[ident]
				astIdent, ok := astNode.(*ast.SelectorExpr)
				if ok {
					paramType := data.pkg.TypesInfo.Types[astIdent]
					t := paramType.Type.String()
					if t == "net/http.ResponseWriter" {
						rw = true
					}

				}
			} else if okStar {
				astNode := data.pkg.Decorator.Ast.Nodes[star]
				astStar, ok := astNode.(*ast.StarExpr)
				if ok {
					paramType := data.pkg.TypesInfo.Types[astStar]
					t := paramType.Type.String()
					if t == "*net/http.Request" {
						req = true
					}
				}

			}
		}
		return rw && req
	}
	return false
}

// Optimistically guess if a function is a handler func based on its contents
// Pre-instrumentation function
func InstrumentHandler(n dst.Node, data *InstrumentationData) {
	ident, isIdent := n.(*dst.Ident)
	if isIdent && ident.Obj != nil && ident.Obj.Decl != nil {
		fn, isFn := ident.Obj.Decl.(*dst.FuncDecl)
		if isFn && isHttpHandler(fn.Type.Params.List, data) {
			txnName := "nrTxn"
			body, ok := TraceFunction(data, fn.Body.List, txnName)
			if ok {
				body = append([]dst.Stmt{txnFromCtx(txnName)}, body...)
				fn.Body.List = body
			}
		}
	}
}
