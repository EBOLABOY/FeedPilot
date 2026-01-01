package api

import (
	"os"
	"path/filepath"
	"testing"

	"notebook-podcast-automator/internal/api/testdata"
)

// TestGenerateMockHTTPRRFiles generates all the httprr recording files
// Run with: go test -run TestGenerateMockHTTPRRFiles ./internal/api
func TestGenerateMockHTTPRRFiles(t *testing.T) {
	if os.Getenv("NLM_GENERATE_HTTPRR") != "1" {
		t.Skip("skip generating httprr files (set NLM_GENERATE_HTTPRR=1 to enable)")
	}

	testdataDir := filepath.Join(".", "testdata")

	err := testdata.GenerateMockHTTPRRFiles(testdataDir)
	if err != nil {
		t.Fatalf("Failed to generate mock httprr files: %v", err)
	}

	t.Log("Successfully generated all mock httprr files")
}
