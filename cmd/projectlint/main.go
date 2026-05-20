// Command projectlint runs project-specific Go analyzers.
package main

import (
	"github.com/djosh34/nsx-operator/internal/projectlint"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/multichecker"
)

func main() {
	multichecker.Main(projectAnalyzers()...)
}

func projectAnalyzers() []*analysis.Analyzer {
	analyzers := make([]*analysis.Analyzer, 0, 2)
	analyzers = append(analyzers, projectlint.NoValueReceiversAnalyzer)
	analyzers = append(analyzers, projectlint.NoStructErrorReturnsAnalyzer)

	return analyzers
}
