package campaign

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadAppliesSmokeDefaults(t *testing.T) {
	path := writeSpec(t, `
apiVersion: dlbench.io/v1alpha1
kind: Campaign
metadata:
  name: smoke
image: dlbench-probe:smoke
`)

	spec, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if spec.Scenario != ScenarioFullCycle {
		t.Fatalf("Scenario = %q", spec.Scenario)
	}
	if spec.Warmup.MinCycles != 100 || spec.Warmup.MinDuration.Duration != 30*time.Second {
		t.Fatalf("unexpected warmup defaults: %+v", spec.Warmup)
	}
	if spec.Measurement.Samples != 1000 {
		t.Fatalf("Samples = %d", spec.Measurement.Samples)
	}
}

func TestLoadRejectsUnknownField(t *testing.T) {
	path := writeSpec(t, `
apiVersion: dlbench.io/v1alpha1
kind: Campaign
metadata:
  name: smoke
image: dlbench-probe:smoke
surprise: true
`)

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "field surprise not found") {
		t.Fatalf("Load() error = %v, want unknown field", err)
	}
}

func TestValidationRejectsNonDiagnosticResult(t *testing.T) {
	spec := Spec{
		APIVersion:     APIVersion,
		Kind:           Kind,
		Metadata:       Metadata{Name: "smoke"},
		Classification: "comparable",
		Scenario:       ScenarioFullCycle,
		Image:          "probe:test",
		OutputDir:      "results",
		Container:      Container{NetworkMode: "none"},
		Warmup:         Warmup{MinCycles: 1, MinDuration: Duration{time.Second}},
		Measurement:    Measurement{Samples: 1, CycleTimeout: Duration{time.Second}},
	}
	if err := spec.Validate(); err == nil || !strings.Contains(err.Error(), "classification=diagnostic") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func writeSpec(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "campaign.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
