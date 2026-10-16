package dockerengine

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"docker-lifecycle-bench/internal/campaign"
	"docker-lifecycle-bench/internal/model"
	"docker-lifecycle-bench/internal/monotime"
	"docker-lifecycle-bench/internal/ready"

	"github.com/containerd/errdefs"
	containertypes "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

const (
	managedLabel = "io.dlbench.managed"
	runIDLabel   = "io.dlbench.run-id"
	sampleLabel  = "io.dlbench.sample-id"
)

type Engine struct {
	client *client.Client
	clock  monotime.Clock
	spec   campaign.Spec
}

type SystemInfo struct {
	ServerVersion   string   `json:"serverVersion"`
	OSType          string   `json:"osType"`
	Architecture    string   `json:"architecture"`
	KernelVersion   string   `json:"kernelVersion"`
	CgroupVersion   string   `json:"cgroupVersion"`
	CgroupDriver    string   `json:"cgroupDriver"`
	StorageDriver   string   `json:"storageDriver"`
	DockerRootDir   string   `json:"dockerRootDir"`
	OperatingSystem string   `json:"operatingSystem"`
	CPUs            int      `json:"cpus"`
	MemoryBytes     int64    `json:"memoryBytes"`
	SecurityOptions []string `json:"securityOptions"`
}

func New(spec campaign.Spec) (*Engine, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("create Docker client: %w", err)
	}
	return &Engine{client: cli, clock: monotime.RawClock{}, spec: spec}, nil
}

func (e *Engine) Close() error { return e.client.Close() }

func (e *Engine) SystemInfo(ctx context.Context) (SystemInfo, error) {
	if _, err := e.client.Ping(ctx, client.PingOptions{NegotiateAPIVersion: true}); err != nil {
		return SystemInfo{}, fmt.Errorf("ping Docker: %w", err)
	}
	result, err := e.client.Info(ctx, client.InfoOptions{})
	if err != nil {
		return SystemInfo{}, fmt.Errorf("read Docker info: %w", err)
	}
	info := result.Info
	return SystemInfo{
		ServerVersion:   info.ServerVersion,
		OSType:          info.OSType,
		Architecture:    info.Architecture,
		KernelVersion:   info.KernelVersion,
		CgroupVersion:   info.CgroupVersion,
		CgroupDriver:    info.CgroupDriver,
		StorageDriver:   info.Driver,
		DockerRootDir:   info.DockerRootDir,
		OperatingSystem: info.OperatingSystem,
		CPUs:            info.NCPU,
		MemoryBytes:     info.MemTotal,
		SecurityOptions: info.SecurityOptions,
	}, nil
}

func (e *Engine) ValidateImage(ctx context.Context) error {
	image, err := e.client.ImageInspect(ctx, e.spec.Image)
	if err != nil {
		return fmt.Errorf("inspect preloaded probe image %q: %w", e.spec.Image, err)
	}
	if image.Os != "linux" {
		return fmt.Errorf("probe image OS is %q, want linux", image.Os)
	}
	if image.Architecture != runtime.GOARCH {
		return fmt.Errorf("probe image architecture is %q, host binary architecture is %q; emulation is forbidden", image.Architecture, runtime.GOARCH)
	}
	return nil
}

func (e *Engine) Cycle(ctx context.Context, request model.Request) (operation model.Operation) {
	operation = model.Operation{
		RunID:       request.RunID,
		SampleID:    request.SampleID,
		Phase:       request.Phase,
		Concurrency: request.Concurrency,
		Worker:      request.Worker,
		StartedAt:   time.Now().UTC(),
		Status:      model.StatusError,
	}
	defer func() { operation.FinishedAt = time.Now().UTC() }()

	cycleStart, err := e.clock.NowNS()
	if err != nil {
		return fail(operation, "clock", err)
	}

	config := &containertypes.Config{
		Image:        e.spec.Image,
		AttachStdout: true,
		AttachStderr: true,
		Labels: map[string]string{
			managedLabel: "true",
			runIDLabel:   request.RunID,
			sampleLabel:  fmt.Sprint(request.SampleID),
		},
	}
	hostConfig := &containertypes.HostConfig{
		NetworkMode: containertypes.NetworkMode(e.spec.Container.NetworkMode),
		LogConfig:   containertypes.LogConfig{Type: "none"},
		Resources: containertypes.Resources{
			CpusetCpus: e.spec.Container.CPUSetCPUs,
			CpusetMems: e.spec.Container.CPUSetMems,
		},
	}

	createStart, err := e.clock.NowNS()
	if err != nil {
		return fail(operation, "clock", err)
	}
	created, err := e.client.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config: config, HostConfig: hostConfig, Name: containerName(request),
	})
	createEnd, clockErr := e.clock.NowNS()
	if err != nil {
		return fail(operation, "create", err)
	}
	operation.ContainerID = created.ID
	removed := false
	defer func() {
		if !removed {
			cleanupContext, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			_ = e.removeOwned(cleanupContext, created.ID, request.RunID)
		}
	}()
	if clockErr != nil {
		return fail(operation, "clock", clockErr)
	}
	operation.CreateNS = createEnd - createStart

	attached, err := e.client.ContainerAttach(ctx, created.ID, client.ContainerAttachOptions{
		Stream: true,
		Stdout: true,
		Stderr: true,
	})
	if err != nil {
		return fail(operation, "attach", err)
	}
	defer attached.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = attached.Conn.SetReadDeadline(deadline)
	}

	startBegin, err := e.clock.NowNS()
	if err != nil {
		return fail(operation, "clock", err)
	}
	if _, err := e.client.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		return fail(operation, "start", err)
	}
	startEnd, err := e.clock.NowNS()
	if err != nil {
		return fail(operation, "clock", err)
	}
	operation.StartAPINS = startEnd - startBegin

	frame, err := ready.ReadDockerStream(attached.Reader)
	readyObserved, clockErr := e.clock.NowNS()
	if err != nil {
		return fail(operation, "ready", err)
	}
	if clockErr != nil {
		return fail(operation, "clock", clockErr)
	}
	if frame.MonotonicNS < startBegin || frame.MonotonicNS > readyObserved {
		return fail(operation, "clock-domain", fmt.Errorf(
			"probe timestamp %d is outside host interval [%d, %d]", frame.MonotonicNS, startBegin, readyObserved,
		))
	}
	operation.ProbeMonotonicNS = frame.MonotonicNS
	operation.ProbePID = frame.PID
	operation.StartToReadyNS = frame.MonotonicNS - startBegin
	operation.ReadyObservedNS = readyObserved - startBegin

	waitResult := e.client.ContainerWait(ctx, created.ID, client.ContainerWaitOptions{Condition: containertypes.WaitConditionNotRunning})
	waitStatus, waitErr := waitResult.Result, waitResult.Error
	killBegin, err := e.clock.NowNS()
	if err != nil {
		return fail(operation, "clock", err)
	}
	if _, err := e.client.ContainerKill(ctx, created.ID, client.ContainerKillOptions{Signal: "SIGKILL"}); err != nil {
		return fail(operation, "kill", err)
	}
	killEnd, err := e.clock.NowNS()
	if err != nil {
		return fail(operation, "clock", err)
	}
	operation.KillAPINS = killEnd - killBegin

	waiting := true
	for waiting {
		select {
		case waitResult := <-waitStatus:
			if waitResult.Error != nil {
				return fail(operation, "wait", fmt.Errorf("%s", waitResult.Error.Message))
			}
			waiting = false
		case err, ok := <-waitErr:
			if ok && err != nil {
				return fail(operation, "wait", err)
			}
			waitErr = nil
		case <-ctx.Done():
			return fail(operation, "wait", ctx.Err())
		}
	}
	waitEnd, err := e.clock.NowNS()
	if err != nil {
		return fail(operation, "clock", err)
	}
	operation.DestroyAckNS = waitEnd - killBegin

	removeBegin, err := e.clock.NowNS()
	if err != nil {
		return fail(operation, "clock", err)
	}
	if _, err := e.client.ContainerRemove(ctx, created.ID, client.ContainerRemoveOptions{Force: false}); err != nil {
		return fail(operation, "remove", err)
	}
	removeEnd, err := e.clock.NowNS()
	if err != nil {
		return fail(operation, "clock", err)
	}
	removed = true
	operation.RemoveAckNS = removeEnd - removeBegin
	operation.CycleAckNS = removeEnd - cycleStart
	operation.Status = model.StatusSuccess
	return operation
}

func (e *Engine) removeOwned(ctx context.Context, containerID, runID string) error {
	inspection, err := e.client.ContainerInspect(ctx, containerID, client.ContainerInspectOptions{})
	if errdefs.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	labels := inspection.Container.Config.Labels
	if labels[managedLabel] != "true" || labels[runIDLabel] != runID {
		return fmt.Errorf("refuse cleanup of container %s without matching ownership labels", containerID)
	}
	_, err = e.client.ContainerRemove(ctx, containerID, client.ContainerRemoveOptions{Force: true})
	return err
}

func fail(operation model.Operation, stage string, err error) model.Operation {
	operation.Status = model.StatusError
	operation.ErrorStage = stage
	operation.Error = err.Error()
	return operation
}

func containerName(request model.Request) string {
	runID := strings.Trim(request.RunID, "-")
	if len(runID) > 40 {
		runID = runID[:40]
	}
	return fmt.Sprintf("dlbench-%s-%d", runID, request.SampleID)
}
