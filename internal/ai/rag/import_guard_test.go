package rag_test

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

func TestRagPackageDoesNotImportForbiddenDependencies(t *testing.T) {
	forbidden := []string{
		"SuperBizAgent/internal/infra/milvus",
		"github.com/milvus-io/milvus-sdk-go",
		"SuperBizAgent/utility/client",
	}

	out, err := exec.Command("go", "list", "-json", "SuperBizAgent/internal/ai/rag").Output()
	if err != nil {
		t.Fatalf("go list failed: %v", err)
	}

	var pkg struct {
		Imports []string `json:"Imports"`
	}
	if err := json.Unmarshal(out, &pkg); err != nil {
		t.Fatalf("unmarshal go list output: %v", err)
	}

	for _, imp := range pkg.Imports {
		for _, f := range forbidden {
			if strings.HasPrefix(imp, f) {
				t.Errorf("rag package imports forbidden dependency: %s", imp)
			}
		}
	}
}
