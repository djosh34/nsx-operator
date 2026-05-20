package main

import (
	"testing"

	"github.com/djosh34/nsx-operator/internal/projectlint"
	"golang.org/x/tools/go/analysis"
)

func TestProjectAnalyzersIncludesProjectRules(t *testing.T) {
	analyzers := projectAnalyzers()
	expected := map[*analysis.Analyzer]bool{
		projectlint.NoValueReceiversAnalyzer:     false,
		projectlint.NoStructErrorReturnsAnalyzer: false,
	}

	for _, analyzer := range analyzers {
		if _, ok := expected[analyzer]; ok {
			expected[analyzer] = true
		}
	}

	if len(analyzers) != len(expected) {
		t.Fatalf("expected %d project analyzers, got %d", len(expected), len(analyzers))
	}
	for analyzer, found := range expected {
		if !found {
			t.Fatalf("expected project analyzer %q to be registered", analyzer.Name)
		}
	}
}
