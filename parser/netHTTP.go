package main

import (
	"fmt"
	"go/ast"
	"go/token"

	"github.com/dave/dst"
	"github.com/dave/dst/decorator"
	"github.com/dave/dst/dstutil"
)

const (
	NetHttp = "net/http"

	// Methods that can be instrumented
	HttpHandleFunc = "HandleFunc"
	HttpMuxHandle  = "Handle"
	HttpNewRequest = "NewRequest"
	HttpDo         = "Do"

	// methods cannot be instrumented
	HttpGet      = "Get"
	HttpPost     = "Post"
	HttpHead     = "Head"
	HttpPostForm = "PostForm"

	// default net/http client variable
	HttpDefaultClientVariable = "DefaultClient"
	// default net/http client identifier
	HttpDefaultClient = "DefaultClient"
	// http client type
	HttpClientType = `*net/http.Client`
	HttpPath       = `net/http`
)

func typeOfIdent(ident *dst.Ident, pkg *decorator.Package) string {
	if ident == nil {
		return ""
	}
	astNode := pkg.Decorator.Ast.Nodes[ident]
	var astIdent *ast.Ident
	switch v := astNode.(type) {
	case *ast.SelectorExpr:
		astIdent = v.Sel
	case *ast.Ident:
		astIdent = v
	default:
		return ""
	}

	uses, ok := pkg.TypesInfo.Uses[astIdent]
	if ok {
		if uses.Pkg() != nil {
			return uses.Pkg().Path()
		}
	}
	return ""
}

// GetNetHttpClientVariableName looks for an http client in the call expression n. If it finds one, the name
// of the variable containing the client will be returned as a string.
func GetNetHttpClientVariableName(n *dst.CallExpr, pkg *decorator.Package) string {
	if n == nil {
		return ""
	}

	Sel, ok := n.Fun.(*dst.SelectorExpr)
	if ok {
		switch v := Sel.X.(type) {
		case *dst.SelectorExpr:
			path := typeOfIdent(v.Sel, pkg)
			if path == HttpPath {
				return v.Sel.Name
			}
		case *dst.Ident:
			path := typeOfIdent(v, pkg)
			if path == HttpPath {
				return v.Name
			}
		}
	}
	return ""
}

// GetNetHttpMethod gets an http method if one is invoked in the call expression n, and returns the name of it as a string
func GetNetHttpMethod(n *dst.CallExpr, pkg *decorator.Package) string {
	if n == nil {
		return ""
	}

	switch v := n.Fun.(type) {
	case *dst.SelectorExpr:
		path := typeOfIdent(v.Sel, pkg)
		if path == HttpPath {
			return v.Sel.Name
		}
	case *dst.Ident:
		path := typeOfIdent(v, pkg)
		if path == HttpPath {
			return v.Name
		}
	}

	return ""
}

// WrapHandleFunc looks for an instance of http.HandleFunc() and wraps it with a new relic transaction
func WrapHandleFunc(n dst.Node, data *InstrumentationManager, c *dstutil.Cursor) {
	callExpr, ok := n.(*dst.CallExpr)
	if ok {
		funcName := GetNetHttpMethod(callExpr, data.GetDecoratorPackage())
		switch funcName {
		case HttpHandleFunc, HttpMuxHandle:
			if len(callExpr.Args) == 2 {
				// Instrument handle funcs
				oldArgs := callExpr.Args
				callExpr.Args = []dst.Expr{
					&dst.CallExpr{
						Fun: &dst.Ident{
							Name: "WrapHandleFunc",
							Path: newrelicAgentImport,
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
}

func txnFromCtx(fn *dst.FuncDecl, txnVariable string) {
	stmts := make([]dst.Stmt, len(fn.Body.List)+1)
	stmts[0] = &dst.AssignStmt{
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
				Fun: &dst.Ident{
					Name: "FromContext",
					Path: newrelicAgentImport,
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
	for i, stmt := range fn.Body.List {
		stmts[i+1] = stmt
	}
	fn.Body.List = stmts
}

func isHttpHandler(params []*dst.Field, pkg *decorator.Package) bool {
	if len(params) == 2 {
		var rw, req bool
		for _, param := range params {
			ident, ok := param.Type.(*dst.Ident)
			star, okStar := param.Type.(*dst.StarExpr)
			if ok {
				astNode := pkg.Decorator.Ast.Nodes[ident]
				astIdent, ok := astNode.(*ast.SelectorExpr)
				if ok {
					paramType := pkg.TypesInfo.Types[astIdent]
					t := paramType.Type.String()
					if t == "net/http.ResponseWriter" {
						rw = true
					}
				}
			} else if okStar {
				astNode := pkg.Decorator.Ast.Nodes[star]
				astStar, ok := astNode.(*ast.StarExpr)
				if ok {
					paramType := pkg.TypesInfo.Types[astStar]
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

// Recognize if a function is a handler func based on its contents, and inject instrumentation.
// This function discovers entrypoints to tracing for a given transaction and should trace all the way
// down the call chain of the function it is invoked on.
func InstrumentHandleFunction(n dst.Node, data *InstrumentationManager, c *dstutil.Cursor) {
	fn, isFn := n.(*dst.FuncDecl)
	if isFn && isHttpHandler(fn.Type.Params.List, data.GetDecoratorPackage()) {
		txnName := "nrTxn"
		newFn, ok := TraceFunction(data, fn, txnName)
		if ok {
			txnFromCtx(newFn, txnName)
			c.Replace(newFn)
			data.UpdateFunctionDeclaration(newFn)
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
				Fun: &dst.Ident{
					Name: "NewRoundTripper",
					Path: newrelicAgentImport,
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
func InstrumentHttpClient(n dst.Node, data *InstrumentationManager, c *dstutil.Cursor) {
	stmt, ok := n.(*dst.AssignStmt)
	pkg := data.GetDecoratorPackage()
	if ok && len(stmt.Lhs) == 1 {
		clientVar := stmt.Lhs[0]
		astClientVar := pkg.Decorator.Ast.Nodes[clientVar]
		expr, ok := astClientVar.(ast.Expr)
		if ok {
			t := pkg.TypesInfo.TypeOf(expr).String()
			if t == HttpClientType {
				// add new line that adds roundtripper to transports
				c.InsertAfter(injectRoundTripper(clientVar, n.Decorations().After))
				stmt.Decs.After = dst.None
				data.AddImport(newrelicAgentImport)
			}
		}
	}
}

func cannotTraceOutboundHttp(method string, decs *dst.NodeDecs) []string {
	comment := []string{
		fmt.Sprintf("// the \"http.%s()\" net/http method can not be instrumented and its outbound traffic can not be traced", method),
		"// please see these examples of code patterns for external http calls that can be instrumented:",
		"// https://docs.newrelic.com/docs/apm/agents/go-agent/configuration/distributed-tracing-go-agent/#make-http-requests",
	}

	if len(decs.Start.All()) > 0 {
		comment = append(comment, "//")
	}

	return comment
}

// CannotInstrumentHttpMethod is a function that discovers methods of net/http. If that function can not be penetrated by
// instrumentation, it leaves a comment header warning the customer. This function needs no tracing context to work.
func CannotInstrumentHttpMethod(n dst.Node, data *InstrumentationManager, c *dstutil.Cursor) {
	pkg := data.GetDecoratorPackage()
	switch v := n.(type) {
	case *dst.AssignStmt, *dst.ExprStmt:
		dst.Inspect(n, func(n dst.Node) bool {
			c, ok := n.(*dst.CallExpr)
			if ok {
				funcName := GetNetHttpMethod(c, pkg)
				switch funcName {
				case HttpGet, HttpPost, HttpPostForm, HttpHead:
					astCall := pkg.Decorator.Ast.Nodes[c].(*ast.CallExpr)
					if pkg.TypesInfo.TypeOf(astCall).String() == "(resp *net/http.Response, err error)" {
						v.Decorations().Start.Prepend(cannotTraceOutboundHttp(funcName, v.Decorations())...)
					}
				}
				return false
			}
			return true
		})
	}
}

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
				Fun: &dst.Ident{
					Name: "StartExternalSegment",
					Path: newrelicAgentImport,
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

func captureHttpResponse(segmentVariable string, responseVariable dst.Expr) *dst.AssignStmt {
	return &dst.AssignStmt{
		Lhs: []dst.Expr{
			&dst.SelectorExpr{
				X:   dst.NewIdent(segmentVariable),
				Sel: dst.NewIdent("Response"),
			},
		},
		Rhs: []dst.Expr{
			dst.Clone(responseVariable).(dst.Expr),
		},
		Tok: token.ASSIGN,
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

// adds a transaction to the HTTP request context object by creating a line of code that injects it
// equal to calling: newrelic.RequestWithTransactionContext()
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
				Fun: &dst.Ident{
					Name: "RequestWithTransactionContext",
					Path: newrelicAgentImport,
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

func getHttpResponseExpr(data *InstrumentationManager, stmt dst.Stmt) dst.Expr {
	var expression dst.Expr
	pkg := data.GetDecoratorPackage()
	dst.Inspect(stmt, func(n dst.Node) bool {
		switch v := n.(type) {
		case *dst.AssignStmt:
			for _, expr := range v.Lhs {
				astExpr := pkg.Decorator.Ast.Nodes[expr].(ast.Expr)
				t := pkg.TypesInfo.TypeOf(astExpr).String()
				if t == "*net/http.Response" {
					expression = expr
					return false
				}
			}
		}
		return true
	})
	return expression
}

// ExternalHttpCall finds and instruments external net/http calls to the method http.Do.
// It returns a modified function body, and the number of lines that were added.
func ExternalHttpCall(data *InstrumentationManager, stmt dst.Stmt, c *dstutil.Cursor, txnName string) bool {
	if c.Index() < 0 {
		return false
	}
	pkg := data.GetDecoratorPackage()
	var call *dst.CallExpr
	dst.Inspect(stmt, func(n dst.Node) bool {
		switch v := n.(type) {
		case *dst.CallExpr:
			if GetNetHttpMethod(v, pkg) == HttpDo {
				call = v
				return false
			}
		}
		return true
	})
	if call != nil {
		clientVar := GetNetHttpClientVariableName(call, pkg)
		requestObject := call.Args[0]
		if clientVar == HttpDefaultClientVariable {
			// create external segment to wrap calls made with default client
			segmentName := "externalSegment"
			c.InsertBefore(startExternalSegment(requestObject, txnName, segmentName, stmt.Decorations()))
			c.InsertAfter(endExternalSegment(segmentName, stmt.Decorations()))
			responseVar := getHttpResponseExpr(data, stmt)
			data.AddImport(newrelicAgentImport)
			if responseVar != nil {
				c.InsertAfter(captureHttpResponse(segmentName, responseVar))
			}
			return true
		} else {
			c.InsertBefore(addTxnToRequestContext(requestObject, txnName, stmt.Decorations()))
			data.AddImport(newrelicAgentImport)
			return true
		}
	}
	return false
}

// WrapHandleFunction is a function that wraps net/http.HandeFunc() declarations inside of functions
// that are being traced by a transaction.
func WrapNestedHandleFunction(data *InstrumentationManager, stmt dst.Stmt, c *dstutil.Cursor, txnName string) bool {
	wasModified := false
	pkg := data.GetDecoratorPackage()
	dst.Inspect(stmt, func(n dst.Node) bool {
		switch v := n.(type) {
		case *dst.CallExpr:
			callExpr := v
			funcName := GetNetHttpMethod(callExpr, pkg)
			switch funcName {
			case HttpHandleFunc, HttpMuxHandle:
				if len(callExpr.Args) == 2 {
					// Instrument handle funcs
					oldArgs := callExpr.Args
					callExpr.Args = []dst.Expr{
						&dst.CallExpr{
							Fun: &dst.Ident{
								Name: "WrapHandleFunc",
								Path: newrelicAgentImport,
							},
							Args: []dst.Expr{
								&dst.CallExpr{
									Fun: &dst.SelectorExpr{
										X:   dst.NewIdent(txnName),
										Sel: dst.NewIdent("Application"),
									},
								},
								oldArgs[0],
								oldArgs[1],
							},
						},
					}
					wasModified = true
					data.AddImport(newrelicAgentImport)
					return false
				}
			}
		}
		return true
	})

	return wasModified
}
