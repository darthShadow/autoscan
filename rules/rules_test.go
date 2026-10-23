package gorules

import (
	"path/filepath"
	"testing"

	"github.com/quasilyte/go-ruleguard/analyzer"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestLogMessageRules(t *testing.T) {
	rulesPath, err := filepath.Abs("rules.go")
	if err != nil {
		t.Fatalf("resolve rules.go: %v", err)
	}
	analyzer.ForceNewEngine = true
	if err := analyzer.Analyzer.Flags.Set("rules", rulesPath); err != nil {
		t.Fatalf("set ruleguard rules: %v", err)
	}
	testdataPath, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatalf("resolve testdata: %v", err)
	}
	analysistest.Run(t, testdataPath, analyzer.Analyzer, "logmsg")
}
