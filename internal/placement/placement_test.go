package placement

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func writeFixture(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}

func fixture(t *testing.T) filesystem {
	t.Helper()
	fs := filesystem{proc: filepath.Join(t.TempDir(), "proc"), cgroup: filepath.Join(t.TempDir(), "cgroup")}
	for _, group := range []string{"/", "/user.slice", "/system.slice/docker.service", "/system.slice/containerd.service", "/dlbench.slice", "/dlbench.slice/docker-abc.scope"} {
		cpus := "16"
		if group == "/" || group == "/user.slice" {
			cpus = "0-191"
		}
		writeFixture(t, filepath.Join(fs.cgroup, group, "cpuset.cpus.effective"), cpus)
		writeFixture(t, filepath.Join(fs.cgroup, group, "cpuset.mems.effective"), "0-11")
	}
	for _, p := range []struct {
		pid, parent int
		name, group string
	}{
		{100, 1, "dlbench", "/user.slice"},
		{200, 1, "dockerd", "/system.slice/docker.service"},
		{300, 1, "containerd", "/system.slice/containerd.service"},
		{400, 1, "containerd-shim", "/system.slice/containerd.service"},
		{500, 400, "probe", "/dlbench.slice/docker-abc.scope"},
	} {
		base := filepath.Join(fs.proc, strconv.Itoa(p.pid))
		writeFixture(t, filepath.Join(base, "comm"), p.name)
		writeFixture(t, filepath.Join(base, "status"), fmt.Sprintf("PPid:\t%d\n", p.parent))
		writeFixture(t, filepath.Join(base, "cgroup"), "0::"+p.group+"\n")
		writeFixture(t, filepath.Join(base, "task", strconv.Itoa(p.pid), "status"), "Cpus_allowed_list:\t16\n")
	}
	writeFixture(t, filepath.Join(fs.proc, "400/cmdline"), "/usr/bin/containerd-shim-runc-v2\x00-namespace\x00moby\x00")
	return fs
}

func TestSnapshotAndCanary(t *testing.T) {
	fs := fixture(t)
	s := fs.snapshot(16, 100, 200)
	if !s.Passed || len(s.Processes) != 3 {
		t.Fatalf("snapshot: %+v", s)
	}
	canary, err := fs.canary(16, 500, "abc")
	if err != nil || len(canary) != 2 {
		t.Fatalf("canary=%+v err=%v", canary, err)
	}
	if err := VerifyStable(s, fs.snapshot(16, 100, 200)); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotFailsClosed(t *testing.T) {
	for _, test := range []struct{ name, path, value string }{
		{"daemon-unpinned", "cgroup/system.slice/docker.service/cpuset.cpus.effective", "0-191"},
		{"runtime-unpinned", "cgroup/system.slice/containerd.service/cpuset.cpus.effective", "0-191"},
		{"wrong-slice", "cgroup/dlbench.slice/cpuset.cpus.effective", "17"},
		{"empty-mask", "cgroup/dlbench.slice/cpuset.cpus.effective", ""},
		{"extra-harness-thread", "proc/100/task/101/status", "Cpus_allowed_list:\t0-191\n"},
		{"extra-daemon-thread", "proc/200/task/201/status", "Cpus_allowed_list:\t17\n"},
		{"v1-cgroup", "proc/300/cgroup", "1:cpuset:/legacy"},
		{"missing-containerd", "proc/300/comm", "another-daemon"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fs := fixture(t)
			root, suffix := fs.proc, test.path[len("proc/"):]
			if test.path[:6] == "cgroup" {
				root, suffix = fs.cgroup, test.path[len("cgroup/"):]
			}
			writeFixture(t, filepath.Join(root, suffix), test.value)
			if s := fs.snapshot(16, 100, 200); s.Passed || s.Error == "" {
				t.Fatalf("unexpected pass: %+v", s)
			}
		})
	}
}

func TestCanaryRejectsWrongScopeAndUnpinnedShim(t *testing.T) {
	fs := fixture(t)
	if _, err := fs.canary(16, 500, "different"); err == nil {
		t.Fatal("accepted unrelated container scope")
	}
	writeFixture(t, filepath.Join(fs.proc, "400/task/401/status"), "Cpus_allowed_list:\t0-191")
	if _, err := fs.canary(16, 500, "abc"); err == nil {
		t.Fatal("accepted unpinned shim")
	}
}

func TestStableRejectsRestart(t *testing.T) {
	fs := fixture(t)
	before, after := fs.snapshot(16, 100, 200), fs.snapshot(16, 100, 200)
	after.Processes[1].PID = 201
	if err := VerifyStable(before, after); err == nil {
		t.Fatal("accepted daemon restart")
	}
}

func TestSingleCPUListsAndCgroupPaths(t *testing.T) {
	for _, value := range []string{"0", "0-0", "0\n"} {
		if !singleton(value, 0) {
			t.Fatal(value)
		}
	}
	for _, value := range []string{"", "0-1", "0,1", "1"} {
		if singleton(value, 0) {
			t.Fatal(value)
		}
	}
	for _, value := range []string{"1:cpuset:/", "0::/../other", "0::relative"} {
		if _, err := unifiedPath(value); err == nil {
			t.Fatal(value)
		}
	}
}
