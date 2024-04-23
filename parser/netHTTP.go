package main

import (
	"go/ast"
)

func isNetHTTP(n *ast.CallExpr) bool {
	sel, ok := n.Fun.(*ast.SelectorExpr)
	if ok {
		indent, ok := sel.X.(*ast.Ident)
		if ok {
			return indent.Name == "http"
		}
	}
	return false
}

func isHandleFunc(n *ast.CallExpr) bool {
	sel, ok := n.Fun.(*ast.SelectorExpr)
	if ok {
		return sel.Sel.Name == "HandleFunc"
	}

	return false
}

func GetHandleFuncs(n ast.Node, data *InstrumentationData) string {
	var handler string
	callExpr, ok := n.(*ast.CallExpr)
	if ok && isNetHTTP(callExpr) {
		if isHandleFunc(callExpr) {
			if len(callExpr.Args) == 2 {
				// Capture name of handle funcs for deeper instrumentation
				handleFunc, ok := callExpr.Args[1].(*ast.Ident)
				if ok {
					handler = handleFunc.Name
				}
				return handler
			}
		}
	}
	return ""
}

func InstrumentHandleFunc(n ast.Node, data *InstrumentationData) string {
	callExpr, ok := n.(*ast.CallExpr)
	if ok && isNetHTTP(callExpr) {
		if isHandleFunc(callExpr) {
			if len(callExpr.Args) == 2 {
				// Instrument handle funcs
				oldArgs := callExpr.Args
				callExpr.Args = []ast.Expr{
					&ast.CallExpr{
						Fun: &ast.SelectorExpr{
							X: &ast.Ident{
								Name: "newrelic",
							},
							Sel: &ast.Ident{
								Name: "WrapHandleFunc",
							},
						},
						Lparen: 66,
						Args: []ast.Expr{
							&ast.Ident{
								Name: data.AgentVariableName,
							},
							oldArgs[0],
							oldArgs[1],
						},
						Ellipsis: 0,
					},
				}
			}
		}
	}
	return ""
}
