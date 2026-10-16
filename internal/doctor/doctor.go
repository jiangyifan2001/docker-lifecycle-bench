package doctor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"docker-lifecycle-bench/internal/dockerengine"
)

type Check struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
	Passed   bool   `json:"passed"`
	Detail   string `json:"detail"`
}

type Evidence struct {
	Uname          string `json:"uname,omitempty"`
	LSCPU          string `json:"lscpu,omitempty"`
	NUMAHardware   string `json:"numaHardware,omitempty"`
	Virtualization string `json:"virtualization,omitempty"`
}

type Report struct {
	CollectedAt time.Time               `json:"collectedAt"`
	GOOS        string                  `json:"goos"`
	GOARCH      string                  `json:"goarch"`
	Eligible    bool                    `json:"eligible"`
	Checks      []Check                 `json:"checks"`
	Docker      dockerengine.SystemInfo `json:"docker"`
	Evidence    Evidence                `json:"evidence"`
}

type DockerInspector interface {
	SystemInfo(context.Context) (dockerengine.SystemInfo, error)
}

func Collect(ctx context.Context, docker DockerInspector) Report {
	report := Report{
		CollectedAt: time.Now().UTC(),
		GOOS:        runtime.GOOS,
		GOARCH:      runtime.GOARCH,
	}
	report.Checks = append(report.Checks, Check{
		Name: "linux-host", Required: true, Passed: runtime.GOOS == "linux",
		Detail: fmt.Sprintf("host binary reports GOOS=%s", runtime.GOOS),
	})
	report.Checks = append(report.Checks, Check{
		Name: "native-amd64", Required: true, Passed: runtime.GOARCH == "amd64",
		Detail: fmt.Sprintf("phase 0 Hygon target requires GOARCH=amd64; got %s", runtime.GOARCH),
	})

	report.Evidence.Uname, _ = commandOutput(ctx, "uname", "-a")
	report.Evidence.LSCPU, _ = commandOutput(ctx, "lscpu")
	report.Evidence.NUMAHardware, _ = commandOutput(ctx, "numactl", "--hardware")

	virtualization, virtualized, virtErr := virtualizationStatus(ctx)
	report.Evidence.Virtualization = virtualization
	report.Checks = append(report.Checks, Check{
		Name: "bare-metal", Required: true, Passed: virtErr == nil && !virtualized,
		Detail: virtualization,
	})

	_, cgroupErr := os.Stat("/sys/fs/cgroup/cgroup.controllers")
	report.Checks = append(report.Checks, Check{
		Name: "cgroup-v2", Required: true, Passed: cgroupErr == nil,
		Detail: detailForError("/sys/fs/cgroup/cgroup.controllers is present", cgroupErr),
	})

	dockerInfo, dockerErr := docker.SystemInfo(ctx)
	report.Docker = dockerInfo
	report.Checks = append(report.Checks, Check{
		Name: "docker-access", Required: true, Passed: dockerErr == nil,
		Detail: detailForError("Docker daemon is reachable", dockerErr),
	})
	rootless := false
	for _, option := range dockerInfo.SecurityOptions {
		if strings.Contains(strings.ToLower(option), "rootless") {
			rootless = true
		}
	}
	report.Checks = append(report.Checks, Check{
		Name: "rootful-docker", Required: true, Passed: dockerErr == nil && !rootless,
		Detail: map[bool]string{true: "Docker is rootless", false: "Docker is rootful"}[rootless],
	})
	report.Checks = append(report.Checks, Check{
		Name: "numa-topology-recorded", Required: false, Passed: report.Evidence.NUMAHardware != "",
		Detail: map[bool]string{true: "numactl --hardware succeeded", false: "numactl is unavailable or failed"}[report.Evidence.NUMAHardware != ""],
	})

	report.Eligible = true
	for _, check := range report.Checks {
		if check.Required && !check.Passed {
			report.Eligible = false
		}
	}
	return report
}

func commandOutput(ctx context.Context, name string, args ...string) (string, error) {
	output, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return strings.TrimSpace(string(output)), err
}

func virtualizationStatus(ctx context.Context) (detail string, virtualized bool, err error) {
	command := exec.CommandContext(ctx, "systemd-detect-virt")
	output, commandErr := command.CombinedOutput()
	detail = strings.TrimSpace(string(output))
	if commandErr == nil {
		if detail == "none" || detail == "" {
			return "systemd-detect-virt reports none", false, nil
		}
		return "systemd-detect-virt reports " + detail, true, nil
	}
	var exitError *exec.ExitError
	if errors.As(commandErr, &exitError) && exitError.ExitCode() == 1 {
		return "systemd-detect-virt reports bare metal", false, nil
	}
	if detail == "" {
		detail = commandErr.Error()
	}
	return detail, false, commandErr
}

func detailForError(success string, err error) string {
	if err == nil {
		return success
	}
	return err.Error()
}
