// Package projectlint contains project-specific Go analysis checks.
package projectlint

import (
	"bytes"
	"go/ast"
	"go/printer"
	"go/token"
	"go/types"
	"reflect"
	"strings"

	"golang.org/x/tools/go/analysis"
)

const (
	valueReceiverRule     = "value-receiver"
	structErrorReturnRule = "struct-error-return"
)

var emptyAnalyzerResult = struct{}{}

// NoValueReceiversAnalyzer reports method declarations whose receiver is not a pointer.
var NoValueReceiversAnalyzer = &analysis.Analyzer{
	Name:       "novaluereceivers",
	Doc:        "reports method declarations whose receiver is not a pointer",
	Run:        runNoValueReceivers,
	ResultType: reflect.TypeOf(emptyAnalyzerResult),
}

// NoStructErrorReturnsAnalyzer reports functions returning a named struct value and error.
var NoStructErrorReturnsAnalyzer = &analysis.Analyzer{
	Name:       "nostructerrorreturns",
	Doc:        "reports functions returning a named struct value and error",
	Run:        runNoStructErrorReturns,
	ResultType: reflect.TypeOf(emptyAnalyzerResult),
}

func runNoValueReceivers(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv == nil {
				continue
			}

			for _, receiver := range function.Recv.List {
				if _, receiverIsPointer := receiver.Type.(*ast.StarExpr); receiverIsPointer {
					continue
				}
				if hasAllowDirective(function.Doc, valueReceiverRule) {
					continue
				}

				receiverName := receiverIdentifier(receiver)
				receiverType := expressionString(pass.Fset, receiver.Type)
				pass.Reportf(
					function.Pos(),
					"method receiver must be pointer receiver: use func (%s *%s), not func (%s %s)",
					receiverName,
					receiverType,
					receiverName,
					receiverType,
				)
			}
		}
	}

	return emptyAnalyzerResult, nil
}

func runNoStructErrorReturns(pass *analysis.Pass) (any, error) {
	errorObject := types.Universe.Lookup("error")
	if errorObject == nil {
		return emptyAnalyzerResult, nil
	}

	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Type.Results == nil {
				continue
			}

			results := expandedResultTypes(function.Type.Results)
			if len(results) != 2 {
				continue
			}
			if _, resultIsPointer := results[0].(*ast.StarExpr); resultIsPointer {
				continue
			}
			if !types.Identical(pass.TypesInfo.TypeOf(results[1]), errorObject.Type()) {
				continue
			}

			typeName, ok := namedStructTypeName(pass.TypesInfo.TypeOf(results[0]))
			if !ok {
				continue
			}
			if hasAllowDirective(function.Doc, structErrorReturnRule) {
				continue
			}

			pass.Reportf(
				function.Pos(),
				"functions returning a struct and error must return *Struct, error so error paths can return nil, err: use *%s, error",
				typeName,
			)
		}
	}

	return emptyAnalyzerResult, nil
}

func expandedResultTypes(results *ast.FieldList) []ast.Expr {
	expanded := make([]ast.Expr, 0, len(results.List))
	for _, result := range results.List {
		resultCount := len(result.Names)
		if resultCount == 0 {
			resultCount = 1
		}

		for range resultCount {
			expanded = append(expanded, result.Type)
		}
	}

	return expanded
}

func namedStructTypeName(goType types.Type) (string, bool) {
	if goType == nil {
		return "", false
	}
	if _, ok := goType.Underlying().(*types.Struct); !ok {
		return "", false
	}

	switch concrete := goType.(type) {
	case *types.Named:
		return concrete.Obj().Name(), true
	case *types.Alias:
		return concrete.Obj().Name(), true
	default:
		return "", false
	}
}

func receiverIdentifier(receiver *ast.Field) string {
	if len(receiver.Names) == 0 {
		return "receiver"
	}

	return receiver.Names[0].Name
}

func expressionString(fileSet *token.FileSet, expression ast.Expr) string {
	var buffer bytes.Buffer
	err := printer.Fprint(&buffer, fileSet, expression)
	if err != nil {
		return "<unknown>"
	}

	return buffer.String()
}

func hasAllowDirective(commentGroup *ast.CommentGroup, rule string) bool {
	if commentGroup == nil {
		return false
	}

	for _, comment := range commentGroup.List {
		text := directiveText(comment.Text)
		fields := strings.Fields(text)
		if len(fields) == 0 || fields[0] != "projectlint:allow" {
			continue
		}
		if len(fields) < 2 || fields[1] != rule {
			continue
		}

		return len(fields) > 2
	}

	return false
}

func directiveText(comment string) string {
	text := strings.TrimSpace(comment)
	text = strings.TrimPrefix(text, "//")
	text = strings.TrimPrefix(text, "/*")
	text = strings.TrimSuffix(text, "*/")

	return strings.TrimSpace(text)
}
