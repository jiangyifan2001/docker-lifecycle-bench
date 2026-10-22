//go:build linux

package placement

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"

	"golang.org/x/sys/unix"
)

func TestPinHarnessInChild(t *testing.T) {
	// Only the child changes its own affinity; the parent runner is untouched.
	if value := os.Getenv("DLBENCH_AFFINITY_TEST_CPU"); value != "" {
		cpu, err := strconv.Atoi(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := PinHarness(cpu); err != nil {
			t.Fatal(err)
		}
		var wait sync.WaitGroup
		release := make(chan struct{})
		for range 8 {
			wait.Add(1)
			go func() {
				runtime.LockOSThread()
				defer runtime.UnlockOSThread()
				wait.Done()
				<-release
			}()
		}
		wait.Wait()
		_, err = host.threads(os.Getpid(), cpu)
		close(release)
		if err != nil {
			t.Fatal(err)
		}
		return
	}
	var mask unix.CPUSet
	if err := unix.SchedGetaffinity(0, &mask); err != nil {
		t.Fatal(err)
	}
	cpu := 0
	for !mask.IsSet(cpu) {
		cpu++
	}
	child := exec.Command(os.Args[0], "-test.run=^TestPinHarnessInChild$")
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, reexecKey+"=") {
			child.Env = append(child.Env, entry)
		}
	}
	child.Env = append(child.Env, "DLBENCH_AFFINITY_TEST_CPU="+strconv.Itoa(cpu))
	if output, err := child.CombinedOutput(); err != nil {
		t.Fatalf("child: %v\n%s", err, output)
	}
}

func TestRejectRemoteDocker(t *testing.T) {
	for _, endpoint := range []string{"tcp://localhost:2375", "ssh://host", "unix://relative"} {
		if _, err := dockerPeer(context.Background(), endpoint); err == nil {
			t.Fatal(endpoint)
		}
	}
}
