package main

import "github.com/dave/dst"

func isNetHTTP(n *dst.CallExpr) bool {
	sel, ok := n.Fun.(*dst.SelectorExpr)
	if ok {
		indent, ok := sel.X.(*dst.Ident)
		if ok {
			return indent.Name == "http"
		}
	}
	return false
}

func isHandleFunc(n *dst.CallExpr) bool {
	sel, ok := n.Fun.(*dst.SelectorExpr)
	if ok {
		return sel.Sel.Name == "HandleFunc"
	}

	return false
}

func InstrumentHandleFunc(n dst.Node, data *InstrumentationData) string {
	var handler string
	callExpr, ok := n.(*dst.CallExpr)
	if ok && isNetHTTP(callExpr) && isHandleFunc(callExpr) && len(callExpr.Args) == 2 {
		// Capture name of handle funcs for deeper instrumentation
		handleFunc, ok := callExpr.Args[1].(*dst.Ident)
		if ok {
			handler = handleFunc.Name
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
	return handler
}
