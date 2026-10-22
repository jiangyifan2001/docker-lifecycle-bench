//go:build linux

package placement

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

const reexecKey = "DLBENCH_SINGLE_CPU_REEXEC"

// Re-exec from a pinned thread so all threads in the new Go runtime inherit the
// mask. Iterating existing TIDs alone races thread creation. Call before opening
// result bundles or Docker connections. Never change another process's mask.
func PinHarness(cpu int) error {
	var mask unix.CPUSet
	if cpu < 0 || cpu >= len(mask)*64 {
		return fmt.Errorf("logical CPU %d exceeds supported affinity mask", cpu)
	}
	if _, err := host.threads(os.Getpid(), cpu); err == nil {
		runtime.GOMAXPROCS(1)
		return nil
	}
	if os.Getenv(reexecKey) != "" {
		return fmt.Errorf("harness threads are not all pinned after exec; refusing a re-exec loop")
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := unix.SchedGetaffinity(0, &mask); err != nil {
		return err
	}
	if !mask.IsSet(cpu) {
		return fmt.Errorf("CPU %d is not in the harness's current allowed CPU set", cpu)
	}
	original := mask
	mask.Zero()
	mask.Set(cpu)
	if err := unix.SchedSetaffinity(0, &mask); err != nil {
		return err
	}
	err = unix.Exec(executable, os.Args, append(os.Environ(), reexecKey+"="+strconv.Itoa(cpu)))
	_ = unix.SchedSetaffinity(0, &original) // Only reached if exec failed.
	return err
}

// Read-only qualification; reject TCP/SSH proxies. PID 1 activation is supported
// only for the standard docker.socket -> docker.service mapping.
func CheckHost(ctx context.Context, cpu int, dockerHost string) Snapshot {
	pid, err := dockerPeer(ctx, dockerHost)
	if err != nil {
		return Snapshot{CPU: cpu, CollectedAt: time.Now().UTC(), Error: err.Error()}
	}
	return host.snapshot(cpu, os.Getpid(), pid)
}

func dockerPeer(ctx context.Context, endpoint string) (int, error) {
	if !strings.HasPrefix(endpoint, "unix:///") {
		return 0, fmt.Errorf("single-CPU mode requires a direct local Unix Docker socket, got %q", endpoint)
	}
	path := strings.TrimPrefix(endpoint, "unix://")
	connection, err := (&net.Dialer{}).DialContext(ctx, "unix", path)
	if err != nil {
		return 0, err
	}
	defer connection.Close()
	conn, ok := connection.(*net.UnixConn)
	if !ok {
		return 0, fmt.Errorf("Docker connection is not a Unix socket")
	}
	raw, err := conn.SyscallConn()
	if err != nil {
		return 0, err
	}
	var credentials *unix.Ucred
	var credentialErr error
	if err := raw.Control(func(fd uintptr) {
		credentials, credentialErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); err != nil {
		return 0, err
	}
	if credentialErr != nil {
		return 0, credentialErr
	}
	if credentials.Uid != 0 {
		return 0, fmt.Errorf("Docker socket peer is not rootful")
	}
	pid := int(credentials.Pid)
	name, err := readTrim(filepath.Join("/proc", strconv.Itoa(pid), "comm"))
	if err != nil {
		return 0, err
	}
	if name == "dockerd" {
		return pid, nil
	}
	if pid != 1 || name != "systemd" {
		return 0, fmt.Errorf("Docker socket peer PID %d (%s) is not dockerd or supported socket activation", pid, name)
	}
	show := func(unit, property string) (string, error) {
		out, err := exec.CommandContext(ctx, "systemctl", "show", unit, "--property="+property, "--value").Output()
		return strings.TrimSpace(string(out)), err
	}
	listen, err := show("docker.socket", "Listen")
	if err != nil {
		return 0, err
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return 0, err
	}
	matched := false
	for _, line := range strings.Split(listen, "\n") {
		if !strings.HasSuffix(line, " (Stream)") {
			continue
		}
		candidate, err := filepath.EvalSymlinks(strings.TrimSuffix(line, " (Stream)"))
		if err == nil && candidate == canonical {
			matched = true
		}
	}
	triggers, err := show("docker.socket", "Triggers")
	if err != nil {
		return 0, err
	}
	mainPID, err := show("docker.service", "MainPID")
	if err != nil {
		return 0, err
	}
	name, err = readTrim(filepath.Join("/proc", mainPID, "comm"))
	if !matched || triggers != "docker.service" || err != nil || name != "dockerd" {
		return 0, fmt.Errorf("cannot verify docker.socket activation of local docker.service")
	}
	return 1, nil
}
