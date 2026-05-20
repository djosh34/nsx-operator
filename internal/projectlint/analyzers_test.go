package projectlint

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestNoValueReceivers(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, NoValueReceiversAnalyzer, "novaluereceivers")
}

func TestNoStructErrorReturns(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, NoStructErrorReturnsAnalyzer, "nostructerrorreturns")
}
