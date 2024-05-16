package main

import (
	"fmt"
	"go/ast"
	"go/token"

	"github.com/dave/dst"
	"github.com/dave/dst/decorator"
)

const (
	NetHttp = "net/http"

	// Methods that can be instrumented
	HttpHandleFunc = "HandleFunc"
	HttpNewRequest = "NewRequest"
	HttpDo         = "Do"

	// methods cannot be instrumented
	HttpGet      = "Get"
	HttpPost     = "Post"
	HttpHead     = "Head"
	HttpPostForm = "PostForm"

	// default net/http client variable
	HttpDefaultClientVariable = "http.DefaultClient"
	// default net/http client identifier
	HttpDefaultClient = "DefaultClient"
	// http client type
	HttpClientType = `*net/http.Client`
)

// returns (method name, client variable name)
func getHttpMethodAndClient(n *dst.CallExpr) (string, string) {
	ident, ok := n.Fun.(*dst.Ident)
	if ok && ident.Path == NetHttp {
		return ident.Name, ""
	}
	sel, ok := n.Fun.(*dst.SelectorExpr)
	if ok && sel.Sel.Name == HttpDo {
		ident, ok := sel.X.(*dst.Ident)
		if ok {
			if ident.Path == NetHttp && ident.Name == HttpDefaultClient {
				return sel.Sel.Name, HttpDefaultClientVariable
			}
		}
		return sel.Sel.Name, ""
	}

	return "", ""
}

func InstrumentHandleFunc(n dst.Node, data *InstrumentationData, parent ParentFunction) {
	callExpr, ok := n.(*dst.CallExpr)
	if ok {
		funcName, _ := getHttpMethodAndClient(callExpr)
		if funcName == HttpHandleFunc && len(callExpr.Args) == 2 {
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
func InstrumentHandler(n dst.Node, data *InstrumentationData, parent ParentFunction) {
	ident, isIdent := n.(*dst.Ident)
	if isIdent && ident.Obj != nil && ident.Obj.Decl != nil {
		fn, isFn := ident.Obj.Decl.(*dst.FuncDecl)
		if isFn && isHttpHandler(fn.Type.Params.List, data) {
			txnName := newRelicTxnVariableName
			body, ok := TraceFunction(data, fn.Body.List, txnName)
			if ok {
				body = append([]dst.Stmt{txnFromCtx(txnName)}, body...)
				fn.Body.List = body
			}
		}
	}
}

func injectRoundTripper(clientVariable dst.Expr, spacingAfter dst.SpaceType) *dst.AssignStmt {
	return &dst.AssignStmt{
		Lhs: []dst.Expr{
			&dst.SelectorExpr{
				X:   dst.Clone(clientVariable).(dst.Expr),
				Sel: dst.NewIdent("Transport"),
			},
		},
		Tok: token.ASSIGN,
		Rhs: []dst.Expr{
			&dst.CallExpr{
				Fun: &dst.SelectorExpr{
					X:   dst.NewIdent("newrelic"),
					Sel: dst.NewIdent("NewRoundTripper"),
				},
				Args: []dst.Expr{
					&dst.SelectorExpr{
						X:   dst.Clone(clientVariable).(dst.Expr),
						Sel: dst.NewIdent("Transport"),
					},
				},
			},
		},
		Decs: dst.AssignStmtDecorations{
			NodeDecs: dst.NodeDecs{
				After: spacingAfter,
			},
		},
	}
}

// InstrumentHttpClient automatically injects a newrelic roundtripper into any newly created http client
// Returns modified function body, and number of lines added ahead of the current index
func InstrumentHttpClient(n dst.Node, data *InstrumentationData, parent ParentFunction) {
	stmt, ok := n.(*dst.AssignStmt)
	if ok && len(stmt.Lhs) == 1 {
		clientVar := stmt.Lhs[0]
		astClientVar := data.pkg.Decorator.Ast.Nodes[clientVar]
		expr, ok := astClientVar.(ast.Expr)
		if ok && data.pkg.TypesInfo.TypeOf(expr).String() == HttpClientType {
			// add new line that adds roundtripper to transport
			if parent.cursor.Index() > 0 {
				parent.cursor.InsertAfter(injectRoundTripper(clientVar, n.Decorations().After))
				stmt.Decs.After = dst.None
			}
		}
	}
}

func cannotTraceOutboundHttp(method string) []string {
	return []string{
		fmt.Sprintf("// the \"%s\" net/http method can not be instrumented and its outbound traffic can not be traced", method),
		"// please see these examples of code patterns for external http calls that can be instrumented:",
		"// https://docs.newrelic.com/docs/apm/agents/go-agent/configuration/distributed-tracing-go-agent/#make-http-requests",
		"//",
	}
}

func CannotInstrumentHttpMethod(n dst.Node, data *InstrumentationData, parent ParentFunction) {
	var call *dst.CallExpr
	stmt, ok := n.(*dst.AssignStmt)
	if ok {
		if len(stmt.Rhs) == 1 {
			call, ok = stmt.Rhs[0].(*dst.CallExpr)
			if ok {
				funcName, _ := getHttpMethodAndClient(call)
				if funcName != "" {
					switch funcName {
					case HttpGet, HttpPost, HttpPostForm, HttpHead:
						n.Decorations().Start.Prepend(cannotTraceOutboundHttp(funcName)...)
					}
				}
			}
		}
	}
}

// TODO
func startExternalSegment(request dst.Expr, txnVar, segmentVar string, nodeDecs *dst.NodeDecs) *dst.AssignStmt {
	// copy all preceeding decorations from the previous node
	decs := dst.AssignStmtDecorations{
		NodeDecs: dst.NodeDecs{
			Before: nodeDecs.Before,
			Start:  nodeDecs.Start,
		},
	}

	// Clear the decs from the previous node since they are being moved up
	nodeDecs.Before = dst.None
	nodeDecs.Start.Clear()

	return &dst.AssignStmt{
		Tok: token.DEFINE,
		Lhs: []dst.Expr{
			dst.NewIdent(segmentVar),
		},
		Rhs: []dst.Expr{
			&dst.CallExpr{
				Fun: &dst.SelectorExpr{
					X:   dst.NewIdent("newrelic"),
					Sel: dst.NewIdent("StartExternalSegment"),
				},
				Args: []dst.Expr{
					dst.NewIdent(txnVar),
					dst.Clone(request).(dst.Expr),
				},
			},
		},
		Decs: decs,
	}
}

func endExternalSegment(segmentName string, nodeDecs *dst.NodeDecs) *dst.ExprStmt {
	decs := dst.ExprStmtDecorations{
		NodeDecs: dst.NodeDecs{
			After: nodeDecs.After,
			End:   nodeDecs.End,
		},
	}

	nodeDecs.After = dst.None
	nodeDecs.End.Clear()

	return &dst.ExprStmt{
		X: &dst.CallExpr{
			Fun: &dst.SelectorExpr{
				X:   dst.NewIdent(segmentName),
				Sel: dst.NewIdent("End"),
			},
		},
		Decs: decs,
	}
}

func addTxnToRequestContext(request dst.Expr, txnVar string, nodeDecs *dst.NodeDecs) *dst.AssignStmt {
	// Copy all decs above prior statement into this one
	decs := dst.AssignStmtDecorations{
		NodeDecs: dst.NodeDecs{
			Before: nodeDecs.Before,
			Start:  nodeDecs.Start,
		},
	}

	// Clear the decs from the previous node since they are being moved up
	nodeDecs.Before = dst.None
	nodeDecs.Start.Clear()

	return &dst.AssignStmt{
		Tok: token.ASSIGN,
		Lhs: []dst.Expr{dst.Clone(request).(dst.Expr)},
		Rhs: []dst.Expr{
			&dst.CallExpr{
				Fun: &dst.SelectorExpr{
					X:   dst.NewIdent("newrelic"),
					Sel: dst.NewIdent("RequestWithTransactionContext"),
				},
				Args: []dst.Expr{
					dst.Clone(request).(dst.Expr),
					dst.NewIdent(txnVar),
				},
			},
		},
		Decs: decs,
	}
}

// ExternalHttpCall finds and instruments external  net/http calls to the method http.Do.
// It returns a modified function body, and the number of lines that were added.
func ExternalHttpCall(stmt *dst.AssignStmt, pkg *decorator.Package, body []dst.Stmt, bodyIndex int, txnName string) ([]dst.Stmt, int) {
	if len(stmt.Rhs) == 1 {
		call, ok := stmt.Rhs[0].(*dst.CallExpr)
		if ok {
			funcName, clientVar := getHttpMethodAndClient(call)
			if funcName == HttpDo {
				requestObject := call.Args[0]
				if clientVar == HttpDefaultClientVariable {
					// create external segment to wrap calls made with default client
					segmentName := "externalSegment"
					newBody := []dst.Stmt{}
					newBody = append(newBody, body[:bodyIndex-1]...)
					newBody = append(newBody, startExternalSegment(requestObject, txnName, segmentName, &stmt.Decs.NodeDecs))
					newBody = append(newBody, body[bodyIndex-1])
					newBody = append(newBody, endExternalSegment(segmentName, &stmt.Decs.NodeDecs))
					newBody = append(newBody, body[bodyIndex:]...)

					return newBody, 2
				} else {
					// add txn into request object
					newBody := []dst.Stmt{}
					newBody = append(newBody, body[:bodyIndex-1]...)
					newBody = append(newBody, addTxnToRequestContext(requestObject, txnName, &stmt.Decs.NodeDecs))
					newBody = append(newBody, body[bodyIndex-1:]...)

					return newBody, 1
				}
			}
		}
	}
	return nil, 0
}
