package bundle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"docker-lifecycle-bench/internal/model"
)

func TestFinalizePreservesOperationsAndCreatesChecksums(t *testing.T) {
	bundle, runID, err := Create(t.TempDir(), "smoke", time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if err := bundle.WriteOperation(model.Operation{RunID: runID, SampleID: 1, Status: model.StatusSuccess}); err != nil {
		t.Fatal(err)
	}
	if err := bundle.Finalize(Status{RunID: runID, Status: "SUCCESS"}, map[string]string{"result": "ok"}); err != nil {
		t.Fatal(err)
	}

	operations, err := os.ReadFile(filepath.Join(bundle.Path(), "operations.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(operations), `"sampleId":1`) {
		t.Fatalf("operation log was not flushed: %s", operations)
	}
	checksums, err := os.ReadFile(filepath.Join(bundle.Path(), "checksums.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(checksums), "operations.ndjson") || !strings.Contains(string(checksums), "status.json") {
		t.Fatalf("incomplete checksums: %s", checksums)
	}
}
