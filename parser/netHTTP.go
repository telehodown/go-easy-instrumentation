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
	// Find instances of http Do calls ---> new idea, always inject txn into request obj. Always add rtrippr to client, and if default client Do, then just wrap
	if ok && sel.Sel.Name == HttpDo {
		ident, ok := sel.X.(*dst.Ident)
		if ok {
			if ident.Path == NetHttp && ident.Name == HttpDefaultClient {
				return sel.Sel.Name, HttpDefaultClientVariable
			}
		}
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
func startExternalSegment() *dst.AssignStmt {
	return nil
}

// TODO
func endExternalSegment() *dst.CallExpr {
	return nil
}

func addTxnToRequestContext() *dst.AssignStmt {
	return nil
}

func ExternalHttpCall(stmt *dst.AssignStmt, pkg *decorator.Package, body []dst.Stmt, bodyIndex int, txnName string) ([]dst.Stmt, int) {
	if len(stmt.Rhs) == 1 {
		call, ok := stmt.Rhs[0].(*dst.CallExpr)
		if ok {
			funcName, clientVar := getHttpMethodAndClient(call)
			if funcName == HttpDo {
				if clientVar == HttpDefaultClientVariable {
					// create external segment to wrap calls made with default client
				} else {
					// add txn into request objec
				}
			}
		}
	}
	return nil, 0
}
