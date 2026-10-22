package campaign

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	APIVersion        = "dlbench.io/v1alpha1"
	Kind              = "Campaign"
	ScenarioFullCycle = "full-lifecycle"
	SingleCPUSlice    = "dlbench.slice"
)

var campaignName = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

// Duration keeps human-readable duration strings in both YAML and normalized JSON.
type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalText(text []byte) error {
	parsed, err := time.ParseDuration(string(text))
	if err != nil {
		return err
	}
	d.Duration = parsed
	return nil
}

func (d Duration) MarshalText() ([]byte, error) {
	return []byte(d.String()), nil
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

type Metadata struct {
	Name string `yaml:"name" json:"name"`
}

type Container struct {
	NetworkMode  string `yaml:"networkMode" json:"networkMode"`
	CPUSetCPUs   string `yaml:"cpusetCpus" json:"cpusetCpus"`
	CPUSetMems   string `yaml:"cpusetMems" json:"cpusetMems"`
	CgroupParent string `yaml:"cgroupParent,omitempty" json:"cgroupParent,omitempty"`
}

// A pointer on Spec distinguishes CPU 0 from no pinning.
type Placement struct {
	SingleCPU int `yaml:"singleCpu" json:"singleCpu"`
}

func (p *Placement) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode || len(value.Content) != 2 || value.Content[0].Value != "singleCpu" {
		return errors.New("placement must contain exactly one field: singleCpu")
	}
	return value.Content[1].Decode(&p.SingleCPU)
}

type Warmup struct {
	MinCycles   int      `yaml:"minCycles" json:"minCycles"`
	MinDuration Duration `yaml:"minDuration" json:"minDuration"`
}

type Measurement struct {
	Samples      int      `yaml:"samples" json:"samples"`
	CycleTimeout Duration `yaml:"cycleTimeout" json:"cycleTimeout"`
}

type Capacity struct {
	Enabled          bool     `yaml:"enabled" json:"enabled"`
	Workers          []int    `yaml:"workers" json:"workers"`
	DurationPerPoint Duration `yaml:"durationPerPoint" json:"durationPerPoint"`
}

type Spec struct {
	APIVersion     string      `yaml:"apiVersion" json:"apiVersion"`
	Kind           string      `yaml:"kind" json:"kind"`
	Metadata       Metadata    `yaml:"metadata" json:"metadata"`
	Classification string      `yaml:"classification" json:"classification"`
	Scenario       string      `yaml:"scenario" json:"scenario"`
	Image          string      `yaml:"image" json:"image"`
	OutputDir      string      `yaml:"outputDir" json:"outputDir"`
	Container      Container   `yaml:"container" json:"container"`
	Placement      *Placement  `yaml:"placement,omitempty" json:"placement,omitempty"`
	Warmup         Warmup      `yaml:"warmup" json:"warmup"`
	Measurement    Measurement `yaml:"measurement" json:"measurement"`
	Capacity       Capacity    `yaml:"capacity" json:"capacity"`
}

func Load(path string) (Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Spec{}, fmt.Errorf("read campaign spec: %w", err)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var spec Spec
	if err := decoder.Decode(&spec); err != nil {
		return Spec{}, fmt.Errorf("decode campaign spec: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return Spec{}, errors.New("decode campaign spec: multiple YAML documents are not allowed")
		}
		return Spec{}, fmt.Errorf("decode campaign spec: %w", err)
	}

	spec.applyDefaults()
	if spec.Placement != nil {
		if err := spec.SetSingleCPU(spec.Placement.SingleCPU); err != nil {
			return Spec{}, err
		}
	}
	if err := spec.Validate(); err != nil {
		return Spec{}, err
	}
	return spec, nil
}

func (s *Spec) applyDefaults() {
	if s.Classification == "" {
		s.Classification = "diagnostic"
	}
	if s.Scenario == "" {
		s.Scenario = ScenarioFullCycle
	}
	if s.OutputDir == "" {
		s.OutputDir = "results"
	}
	if s.Container.NetworkMode == "" {
		s.Container.NetworkMode = "none"
	}
	if s.Warmup.MinCycles == 0 {
		s.Warmup.MinCycles = 100
	}
	if s.Warmup.MinDuration.Duration == 0 {
		s.Warmup.MinDuration.Duration = 30 * time.Second
	}
	if s.Measurement.Samples == 0 {
		s.Measurement.Samples = 1000
	}
	if s.Measurement.CycleTimeout.Duration == 0 {
		s.Measurement.CycleTimeout.Duration = 30 * time.Second
	}
	if s.Capacity.Enabled && len(s.Capacity.Workers) == 0 {
		s.Capacity.Workers = []int{1, 2, 4, 8}
	}
	if s.Capacity.Enabled && s.Capacity.DurationPerPoint.Duration == 0 {
		s.Capacity.DurationPerPoint.Duration = 30 * time.Second
	}
}

func (s Spec) Validate() error {
	var problems []string
	if s.Placement != nil {
		if s.Placement.SingleCPU < 0 {
			problems = append(problems, "placement.singleCpu must be non-negative")
		}
		if s.Container.CPUSetCPUs != strconv.Itoa(s.Placement.SingleCPU) || s.Container.CgroupParent != SingleCPUSlice {
			problems = append(problems, "single-CPU placement requires matching container.cpusetCpus and cgroupParent=dlbench.slice")
		}
	}
	if s.APIVersion != APIVersion {
		problems = append(problems, fmt.Sprintf("apiVersion must be %q", APIVersion))
	}
	if s.Kind != Kind {
		problems = append(problems, fmt.Sprintf("kind must be %q", Kind))
	}
	if !campaignName.MatchString(s.Metadata.Name) {
		problems = append(problems, "metadata.name must be a lowercase DNS-style name of at most 63 characters")
	}
	if s.Classification != "diagnostic" {
		problems = append(problems, "phase 0 only accepts classification=diagnostic")
	}
	if s.Scenario != ScenarioFullCycle {
		problems = append(problems, "phase 0 only accepts scenario=full-lifecycle")
	}
	if s.Image == "" {
		problems = append(problems, "image is required")
	}
	if s.OutputDir == "" || filepath.Clean(s.OutputDir) == string(filepath.Separator) {
		problems = append(problems, "outputDir must name a non-root directory")
	}
	if s.Container.NetworkMode != "none" {
		problems = append(problems, "phase 0 requires container.networkMode=none")
	}
	if s.Warmup.MinCycles < 1 {
		problems = append(problems, "warmup.minCycles must be at least 1")
	}
	if s.Warmup.MinDuration.Duration <= 0 {
		problems = append(problems, "warmup.minDuration must be positive")
	}
	if s.Measurement.Samples < 1 {
		problems = append(problems, "measurement.samples must be at least 1")
	}
	if s.Measurement.CycleTimeout.Duration <= 0 {
		problems = append(problems, "measurement.cycleTimeout must be positive")
	}
	if s.Capacity.Enabled {
		if s.Capacity.DurationPerPoint.Duration <= 0 {
			problems = append(problems, "capacity.durationPerPoint must be positive")
		}
		seen := make(map[int]bool)
		for _, workers := range s.Capacity.Workers {
			if workers < 1 {
				problems = append(problems, "capacity.workers values must be positive")
			}
			if seen[workers] {
				problems = append(problems, "capacity.workers values must be unique")
			}
			seen[workers] = true
		}
		if len(s.Capacity.Workers) == 0 {
			problems = append(problems, "capacity.workers must not be empty when capacity is enabled")
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("invalid campaign spec: %s", joinProblems(problems))
	}
	return nil
}

func joinProblems(problems []string) string {
	var result bytes.Buffer
	for i, problem := range problems {
		if i > 0 {
			result.WriteString("; ")
		}
		result.WriteString(problem)
	}
	return result.String()
}

func (s Spec) NormalizedJSON() ([]byte, error) {
	return json.MarshalIndent(s, "", "  ")
}

// SetSingleCPU expands the shortcut before validation and recording. Conflicts
// with explicit YAML placement are rejected, never silently overwritten.
func (s *Spec) SetSingleCPU(cpu int) error {
	if cpu < 0 {
		return errors.New("single CPU must be a non-negative logical CPU ID")
	}
	if s.Placement != nil && s.Placement.SingleCPU != cpu {
		return errors.New("--single-cpu conflicts with placement.singleCpu")
	}
	if cpus := s.Container.CPUSetCPUs; cpus != "" && cpus != strconv.Itoa(cpu) {
		return errors.New("single CPU conflicts with container.cpusetCpus (use one CPU ID)")
	}
	if parent := s.Container.CgroupParent; parent != "" && parent != SingleCPUSlice {
		return errors.New("single CPU requires container.cgroupParent=dlbench.slice")
	}
	s.Placement = &Placement{SingleCPU: cpu}
	s.Container.CPUSetCPUs = strconv.Itoa(cpu)
	s.Container.CgroupParent = SingleCPUSlice
	return nil
}
