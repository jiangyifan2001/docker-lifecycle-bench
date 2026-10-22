package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"docker-lifecycle-bench/internal/bundle"
	"docker-lifecycle-bench/internal/campaign"
	"docker-lifecycle-bench/internal/dockerengine"
	"docker-lifecycle-bench/internal/doctor"
	"docker-lifecycle-bench/internal/placement"
	"docker-lifecycle-bench/internal/runner"
	"docker-lifecycle-bench/internal/summary"
)

var (
	version = "dev"
	commit  = "unknown"
)

const (
	exitOK          = 0
	exitInvalidSpec = 2
	exitBlocked     = 3
	exitRunFailed   = 4
)

type manifest struct {
	ToolVersion string `json:"toolVersion"`
	Commit      string `json:"commit"`
	GOOS        string `json:"goos"`
	GOARCH      string `json:"goarch"`
	GoVersion   string `json:"goVersion"`
}

func main() {
	os.Exit(realMain(os.Args[1:]))
}

func realMain(args []string) int {
	if len(args) == 0 {
		usage()
		return exitInvalidSpec
	}
	switch args[0] {
	case "version":
		fmt.Printf("dlbench %s commit=%s go=%s %s/%s\n", version, commit, runtime.Version(), runtime.GOOS, runtime.GOARCH)
		return exitOK
	case "validate":
		return validateCommand(args[1:])
	case "doctor":
		return doctorCommand(args[1:])
	case "run":
		return runCommand(args[1:])
	case "help", "-h", "--help":
		usage()
		return exitOK
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", args[0])
		usage()
		return exitInvalidSpec
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: dlbench <version|validate|doctor|run> [options]")
}

func validateCommand(args []string) int {
	flags := flag.NewFlagSet("validate", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	specPath := flags.String("spec", "", "campaign YAML path")
	flags.Int("single-cpu", -1, "expand the single-logical-CPU diagnostic profile (no host changes)")
	if err := flags.Parse(args); err != nil {
		return exitInvalidSpec
	}
	if *specPath == "" {
		fmt.Fprintln(os.Stderr, "validate: --spec is required")
		return exitInvalidSpec
	}
	spec, err := campaign.Load(*specPath)
	if err == nil {
		err = applyCPUFlag(flags, &spec)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitInvalidSpec
	}
	normalized, err := spec.NormalizedJSON()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitInvalidSpec
	}
	fmt.Println(string(normalized))
	return exitOK
}

func doctorCommand(args []string) int {
	flags := flag.NewFlagSet("doctor", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	if err := flags.Parse(args); err != nil {
		return exitInvalidSpec
	}
	engine, err := dockerengine.New(campaign.Spec{})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitBlocked
	}
	defer engine.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	report := doctor.Collect(ctx, engine)
	printJSON(report)
	if !report.Eligible {
		return exitBlocked
	}
	return exitOK
}

func runCommand(args []string) int {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	specPath := flags.String("spec", "", "campaign YAML path")
	flags.Int("single-cpu", -1, "pin harness and require the prepared Docker/runtime stack on this logical CPU")
	if err := flags.Parse(args); err != nil {
		return exitInvalidSpec
	}
	if *specPath == "" {
		fmt.Fprintln(os.Stderr, "run: --spec is required")
		return exitInvalidSpec
	}
	spec, err := campaign.Load(*specPath)
	if err == nil {
		err = applyCPUFlag(flags, &spec)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitInvalidSpec
	}

	// Pin before opening any files or sockets: on Linux this may exec a fresh
	// copy of this binary so ALL Go runtime threads inherit the affinity mask.
	var pinErr error
	if spec.Placement != nil {
		pinErr = placement.PinHarness(spec.Placement.SingleCPU)
	}
	result, runID, err := bundle.Create(spec.OutputDir, spec.Metadata.Name, time.Now())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitRunFailed
	}
	fmt.Fprintf(os.Stderr, "result bundle: %s\n", result.Path())

	normalized, err := spec.NormalizedJSON()
	if err != nil {
		return finalizeFailure(result, runID, "INVALID_SPEC", err, exitInvalidSpec)
	}
	if err := result.WriteRaw("campaign.normalized.json", append(normalized, '\n')); err != nil {
		return finalizeFailure(result, runID, "RUN_FAILED", err, exitRunFailed)
	}
	if err := result.WriteJSON("manifest.json", manifest{
		ToolVersion: version,
		Commit:      commit,
		GOOS:        runtime.GOOS,
		GOARCH:      runtime.GOARCH,
		GoVersion:   runtime.Version(),
	}); err != nil {
		return finalizeFailure(result, runID, "RUN_FAILED", err, exitRunFailed)
	}

	if pinErr != nil {
		return finalizeFailure(result, runID, "BLOCKED", pinErr, exitBlocked)
	}
	runContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	engine, err := dockerengine.New(spec)
	if err != nil {
		return finalizeFailure(result, runID, "BLOCKED", err, exitBlocked)
	}
	defer engine.Close()

	doctorContext, doctorCancel := context.WithTimeout(runContext, 30*time.Second)
	report := doctor.Collect(doctorContext, engine)
	doctorCancel()
	if err := result.WriteJSON("environment.json", report); err != nil {
		return finalizeFailure(result, runID, "RUN_FAILED", err, exitRunFailed)
	}
	if !report.Eligible {
		return finalizeFailure(result, runID, "BLOCKED", errors.New("required environment checks failed"), exitBlocked)
	}

	imageContext, imageCancel := context.WithTimeout(runContext, 30*time.Second)
	err = engine.ValidateImage(imageContext)
	imageCancel()
	if err != nil {
		return finalizeFailure(result, runID, "BLOCKED", err, exitBlocked)
	}

	var placementBefore placement.Snapshot
	if spec.Placement != nil {
		qualificationContext, cancel := context.WithTimeout(runContext, 30*time.Second)
		evidence, qualificationErr := qualifyPlacement(qualificationContext, spec, runID, engine, report.Docker)
		cancel()
		placementBefore = evidence.Before
		if err := result.WriteJSON("placement.before.json", evidence); err != nil {
			return finalizeFailure(result, runID, "RUN_FAILED", err, exitRunFailed)
		}
		if qualificationErr != nil {
			return finalizeFailure(result, runID, "BLOCKED", qualificationErr, exitBlocked)
		}
	}
	benchmark := runner.Runner{Spec: spec, RunID: runID, Executor: engine, Writer: result}
	operations, runErr := benchmark.Run(runContext)
	if spec.Placement != nil {
		checkContext, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		after := placement.CheckHost(checkContext, spec.Placement.SingleCPU, engine.DockerHost())
		cancel()
		if err := placement.VerifyStable(placementBefore, after); err != nil {
			after.Passed = false
			after.Error = err.Error()
		}
		if !after.Passed {
			runErr = errors.Join(runErr, fmt.Errorf("post-run placement check: %s", after.Error))
		}
		if err := result.WriteJSON("placement.after.json", after); err != nil {
			runErr = errors.Join(runErr, err)
		}
	}
	derivedSummary := summary.Build(operations)
	status := "SUCCESS"
	exitCode := exitOK
	message := ""
	if runErr != nil {
		status = "RUN_FAILED"
		exitCode = exitRunFailed
		message = runErr.Error()
		if errors.Is(runErr, context.Canceled) {
			status = "INTERRUPTED"
		}
	}
	if err := result.Finalize(bundle.Status{
		RunID: runID, Status: status, Message: message, FinishedAt: time.Now().UTC(),
	}, derivedSummary); err != nil {
		fmt.Fprintln(os.Stderr, "finalize result bundle:", err)
		return exitRunFailed
	}
	if runErr != nil {
		fmt.Fprintln(os.Stderr, runErr)
	}
	return exitCode
}

func finalizeFailure(result *bundle.Bundle, runID, status string, cause error, exitCode int) int {
	derivedSummary := summary.Build(nil)
	if err := result.Finalize(bundle.Status{
		RunID: runID, Status: status, Message: cause.Error(), FinishedAt: time.Now().UTC(),
	}, derivedSummary); err != nil {
		fmt.Fprintln(os.Stderr, "finalize result bundle:", err)
	}
	fmt.Fprintln(os.Stderr, cause)
	return exitCode
}

func printJSON(value any) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

func applyCPUFlag(flags *flag.FlagSet, spec *campaign.Spec) (err error) {
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", flags.Args())
	}
	flags.Visit(func(f *flag.Flag) {
		if f.Name == "single-cpu" {
			cpu, parseErr := strconv.Atoi(f.Value.String())
			if parseErr != nil {
				err = parseErr
				return
			}
			err = spec.SetSingleCPU(cpu)
		}
	})
	return err
}

func qualifyPlacement(ctx context.Context, spec campaign.Spec, runID string, engine *dockerengine.Engine, info dockerengine.SystemInfo) (placement.Evidence, error) {
	cpu := spec.Placement.SingleCPU
	evidence := placement.Evidence{Profile: "single-logical-cpu-diagnostic", CPU: cpu, GOMAXPROCS: runtime.GOMAXPROCS(0)}
	if info.CgroupVersion != "2" || info.CgroupDriver != "systemd" || info.DefaultRuntime != "runc" {
		err := errors.New("single-CPU mode requires cgroup v2, systemd cgroup driver, and default runc runtime")
		evidence.Before = placement.Snapshot{CPU: cpu, CollectedAt: time.Now().UTC(), Error: err.Error()}
		return evidence, err
	}
	evidence.Before = placement.CheckHost(ctx, cpu, engine.DockerHost())
	if !evidence.Before.Passed {
		return evidence, errors.New(evidence.Before.Error)
	}
	err := engine.PlacementCanary(ctx, runID, func(pid int, id string) error {
		var err error
		evidence.Canary, err = placement.VerifyCanary(cpu, pid, id)
		return err
	})
	if err != nil {
		evidence.CanaryError = err.Error()
	}
	return evidence, err
}
