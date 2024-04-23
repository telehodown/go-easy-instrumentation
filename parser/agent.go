package main

import (
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/ast/astutil"
)

const (
	agentImport = "github.com/newrelic/go-agent/v3/newrelic"
)

func containsAgentImport(imports []*ast.ImportSpec) bool {
	for _, imp := range imports {
		if imp.Path.Value == agentImport {
			return true
		}
	}
	return false
}

func panicOnError() *ast.IfStmt {
	return &ast.IfStmt{
		If: 27,
		Cond: &ast.BinaryExpr{
			X: &ast.Ident{
				Name: "err",
			},
			OpPos: 34,
			Op:    token.NEQ,
			Y: &ast.Ident{
				Name: "nil",
			},
		},
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				&ast.ExprStmt{
					X: &ast.CallExpr{
						Fun: &ast.Ident{
							Name: "panic",
						},
						Lparen: 49,
						Args: []ast.Expr{
							&ast.Ident{
								Name: "err",
							},
						},
						Ellipsis: 0,
					},
				},
			},
		},
	}
}

func createAgentAST(AppName, AgentVariableName string) []ast.Stmt {
	AppName = "\"" + AppName + "\""

	agentInit := &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.Ident{
				Name: AgentVariableName,
			},
			&ast.Ident{
				Name: "err",
			},
		},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{
			&ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X: &ast.Ident{
						Name: "newrelic",
					},
					Sel: &ast.Ident{
						Name: "NewApplication",
					},
				},
				Lparen: 62,
				Args: []ast.Expr{
					&ast.CallExpr{
						Fun: &ast.SelectorExpr{
							X: &ast.Ident{
								Name: "newrelic",
							},
							Sel: &ast.Ident{
								Name: "ConfigAppName",
							},
						},
						Lparen: 87,
						Args: []ast.Expr{
							&ast.BasicLit{
								ValuePos: 88,
								Kind:     token.STRING,
								Value:    AppName,
							},
						},
						Ellipsis: 0,
					},
					&ast.CallExpr{
						Fun: &ast.SelectorExpr{
							X: &ast.Ident{
								Name: "newrelic",
							},
							Sel: &ast.Ident{
								Name: "ConfigFromEnvironment",
							},
						},
						Lparen:   135,
						Ellipsis: 0,
					},
				},
				Ellipsis: 0,
			},
		},
	}

	return []ast.Stmt{agentInit, panicOnError()}
}

func shutdownAgent(AgentVariableName string) *ast.ExprStmt {
	return &ast.ExprStmt{
		X: &ast.CallExpr{
			Fun: &ast.SelectorExpr{
				X: &ast.Ident{
					Name: AgentVariableName,
				},
				Sel: &ast.Ident{
					Name: "Shutdown",
				},
			},
			Lparen: 39,
			Args: []ast.Expr{
				&ast.BinaryExpr{
					X: &ast.BasicLit{
						ValuePos: 40,
						Kind:     token.INT,
						Value:    "5",
					},
					OpPos: 42,
					Op:    token.MUL,
					Y: &ast.SelectorExpr{
						X: &ast.Ident{
							Name: "time",
						},
						Sel: &ast.Ident{
							Name: "Second",
						},
					},
				},
			},
			Ellipsis: 0,
		},
	}
}

func txnFromCtx() *ast.AssignStmt {
	return &ast.AssignStmt{
		Lhs: []ast.Expr{
			&ast.Ident{
				Name: "txn",
			},
		},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{
			&ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X: &ast.Ident{
						Name: "newrelic",
					},
					Sel: &ast.Ident{
						Name: "FromContext",
					},
				},
				Lparen: 54,
				Args: []ast.Expr{
					&ast.CallExpr{
						Fun: &ast.SelectorExpr{
							X: &ast.Ident{
								Name: "r",
							},
							Sel: &ast.Ident{
								Name: "Context",
							},
						},
						Lparen:   64,
						Ellipsis: 0,
					},
				},
				Ellipsis: 0,
			},
		},
	}
}

func importAgent(fileset *token.FileSet, file *ast.File) string {
	if !containsAgentImport(file.Imports) {
		astutil.AddImport(fileset, file, agentImport)
		return ""
	}
	return ""
}

func InjectAgent(n ast.Node, data *InstrumentationData) string {
	if decl, ok := n.(*ast.FuncDecl); ok {
		// only inject go agent into the main.main function
		if data.AstFile.Name.Name == "main" && decl.Name.Name == "main" {
			importAgent(data.Fset, data.AstFile)
			agentDecl := createAgentAST(data.AppName, data.AgentVariableName)
			decl.Body.List = append(agentDecl, decl.Body.List...)
			decl.Body.List = append(decl.Body.List, shutdownAgent(data.AgentVariableName))
			return ""
		}
	}
	return ""
}

/*
func TraceFunction(n ast.Node, data *InstrumentationData) string {
	if decl, ok := n.(*ast.FuncDecl); ok {
		decl.Body.List = append([]ast.Stmt{txnFromCtx()}, decl.Body.List...)

		// find errors
		ast.Inspect(decl, func(n ast.Node) bool {
			switch t := n.(type) {
			case *ast.AssignStmt:

			}
		})
	}
	return ""
}
*/
