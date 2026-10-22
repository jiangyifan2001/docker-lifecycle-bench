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

func TestSingleCPUConfiguration(t *testing.T) {
	spec, err := Load(writeSpec(t, "apiVersion: dlbench.io/v1alpha1\nkind: Campaign\nmetadata: {name: pin}\nimage: probe:test\nplacement: {singleCpu: 0}\ncontainer: {cpusetMems: '2'}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if spec.Placement == nil || spec.Placement.SingleCPU != 0 || spec.Container.CPUSetCPUs != "0" || spec.Container.CgroupParent != SingleCPUSlice || spec.Container.CPUSetMems != "2" {
		t.Fatalf("%+v", spec)
	}
	if err := spec.SetSingleCPU(1); err == nil {
		t.Fatal("accepted conflicting CPU")
	}
	for _, container := range []Container{{CPUSetCPUs: "0-7"}, {CgroupParent: "system.slice"}} {
		s := Spec{Container: container}
		if err := s.SetSingleCPU(0); err == nil {
			t.Fatal("accepted conflicting placement")
		}
	}
	if err := (&Spec{}).SetSingleCPU(-1); err == nil {
		t.Fatal("accepted negative CPU")
	}
}
