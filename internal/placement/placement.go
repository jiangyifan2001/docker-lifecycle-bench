// Package placement qualifies the opt-in single-logical-CPU diagnostic profile.
// It never writes host cgroups, changes another process, or invokes sudo.
package placement

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Thread struct {
	TID         int    `json:"tid"`
	AllowedCPUs string `json:"allowedCpus"`
}

type Process struct {
	Role          string   `json:"role"`
	PID           int      `json:"pid"`
	ParentPID     int      `json:"parentPid"`
	Cgroup        string   `json:"cgroup"`
	EffectiveCPUs string   `json:"effectiveCpus"`
	EffectiveMems string   `json:"effectiveMems"`
	Threads       []Thread `json:"threads"`
}

type Snapshot struct {
	CollectedAt         time.Time `json:"collectedAt"`
	CPU                 int       `json:"cpu"`
	Passed              bool      `json:"passed"`
	Error               string    `json:"error,omitempty"`
	DockerPeerPID       int       `json:"dockerPeerPid,omitempty"`
	ContainerParent     string    `json:"containerParent"`
	ParentEffectiveCPUs string    `json:"parentEffectiveCpus"`
	ParentEffectiveMems string    `json:"parentEffectiveMems"`
	Processes           []Process `json:"processes"`
}

type Evidence struct {
	Profile     string    `json:"profile"`
	CPU         int       `json:"cpu"`
	GOMAXPROCS  int       `json:"gomaxprocs"`
	Before      Snapshot  `json:"before"`
	Canary      []Process `json:"canary,omitempty"`
	CanaryError string    `json:"canaryError,omitempty"`
}

// Internal seam for synthetic procfs/cgroupfs tests.
type filesystem struct{ proc, cgroup string }

var host = filesystem{proc: "/proc", cgroup: "/sys/fs/cgroup"}

func singleton(value string, cpu int) bool {
	value = strings.TrimSpace(value)
	want := strconv.Itoa(cpu)
	return value == want || value == want+"-"+want
}

func statusField(data, key string) (string, error) {
	for _, line := range strings.Split(data, "\n") {
		if strings.HasPrefix(line, key+":") {
			return strings.TrimSpace(strings.TrimPrefix(line, key+":")), nil
		}
	}
	return "", fmt.Errorf("missing %s in process status", key)
}

func unifiedPath(data string) (string, error) {
	for _, line := range strings.Split(strings.TrimSpace(data), "\n") {
		if strings.HasPrefix(line, "0::/") {
			path := strings.TrimPrefix(line, "0::")
			if filepath.Clean(path) != path {
				return "", fmt.Errorf("non-canonical cgroup path %q", path)
			}
			return path, nil
		}
	}
	return "", fmt.Errorf("no unified cgroup v2 path")
}

func readTrim(path string) (string, error) {
	data, err := os.ReadFile(path)
	return strings.TrimSpace(string(data)), err
}

func (fs filesystem) effective(group, field string) (string, error) {
	// A controller disabled below a parent still constrains descendants. Walk
	// upwards only for absent files, never for empty values or permission errors.
	for {
		value, err := readTrim(filepath.Join(fs.cgroup, group, "cpuset."+field+".effective"))
		if !os.IsNotExist(err) || group == "/" {
			return value, err
		}
		group = filepath.Dir(group)
	}
}

func (fs filesystem) threads(pid, cpu int) ([]Thread, error) {
	base := filepath.Join(fs.proc, strconv.Itoa(pid), "task")
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil, err
	}
	var threads []Thread
	for _, entry := range entries {
		tid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		status, err := readTrim(filepath.Join(base, entry.Name(), "status"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return threads, err
		}
		allowed, err := statusField(status, "Cpus_allowed_list")
		if err != nil {
			return threads, err
		}
		threads = append(threads, Thread{TID: tid, AllowedCPUs: allowed})
		if !singleton(allowed, cpu) {
			return threads, fmt.Errorf("PID %d TID %d allows CPUs %q, want only %d", pid, tid, allowed, cpu)
		}
	}
	if len(threads) == 0 {
		return threads, fmt.Errorf("PID %d has no readable threads", pid)
	}
	return threads, nil
}

func (fs filesystem) process(pid, cpu int, role string, requireCgroup bool) (p Process, err error) {
	p = Process{PID: pid, Role: role}
	base := filepath.Join(fs.proc, strconv.Itoa(pid))
	status, err := readTrim(filepath.Join(base, "status"))
	if err != nil {
		return p, err
	}
	parent, err := statusField(status, "PPid")
	if err != nil {
		return p, err
	}
	p.ParentPID, err = strconv.Atoi(parent)
	if err != nil {
		return p, err
	}
	data, err := readTrim(filepath.Join(base, "cgroup"))
	if err != nil {
		return p, err
	}
	p.Cgroup, err = unifiedPath(data)
	if err != nil {
		return p, err
	}
	p.EffectiveCPUs, err = fs.effective(p.Cgroup, "cpus")
	if err != nil {
		return p, err
	}
	p.EffectiveMems, err = fs.effective(p.Cgroup, "mems")
	if err != nil {
		return p, err
	}
	if requireCgroup && !singleton(p.EffectiveCPUs, cpu) {
		return p, fmt.Errorf("%s PID %d cgroup %s allows CPUs %q, want only %d; administrator preparation required", role, pid, p.Cgroup, p.EffectiveCPUs, cpu)
	}
	p.Threads, err = fs.threads(pid, cpu)
	return p, err
}

// Require ALL local dockerd/containerd instances to be constrained. This avoids
// attributing an unrelated pinned daemon to the socket's backend. The profile
// targets idle, standard runc installations, not heterogeneous shared daemons.
func (fs filesystem) snapshot(cpu, harnessPID, peerPID int) (s Snapshot) {
	s = Snapshot{CPU: cpu, CollectedAt: time.Now().UTC(), DockerPeerPID: peerPID, ContainerParent: "/dlbench.slice"}
	defer func() { s.Passed = s.Error == "" }()
	fail := func(err error) Snapshot { s.Error = err.Error(); return s }
	var err error
	if _, err := os.Stat(filepath.Join(fs.cgroup, s.ContainerParent)); err != nil {
		return fail(fmt.Errorf("prepare dlbench.slice first: %w", err))
	}
	s.ParentEffectiveCPUs, err = fs.effective(s.ContainerParent, "cpus")
	if err != nil {
		return fail(err)
	}
	s.ParentEffectiveMems, err = fs.effective(s.ContainerParent, "mems")
	if err != nil {
		return fail(err)
	}
	if !singleton(s.ParentEffectiveCPUs, cpu) {
		return fail(fmt.Errorf("dlbench.slice effective CPUs are %q, want only %d", s.ParentEffectiveCPUs, cpu))
	}
	p, err := fs.process(harnessPID, cpu, "harness", false)
	s.Processes = append(s.Processes, p)
	if err != nil {
		return fail(err)
	}
	entries, err := os.ReadDir(fs.proc)
	if err != nil {
		return fail(err)
	}
	counts := map[string]int{}
	peerFound := peerPID == 1 // Independently verified socket activation.
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		name, err := readTrim(filepath.Join(fs.proc, entry.Name(), "comm"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fail(err)
		}
		if name != "dockerd" && name != "containerd" {
			continue
		}
		p, err := fs.process(pid, cpu, name, true)
		s.Processes = append(s.Processes, p)
		if err != nil {
			return fail(err)
		}
		counts[name]++
		if name == "dockerd" && pid == peerPID {
			peerFound = true
		}
	}
	if counts["dockerd"] == 0 || counts["containerd"] == 0 || !peerFound {
		return fail(fmt.Errorf("cannot identify local dockerd/containerd serving Docker socket (peer PID %d)", peerPID))
	}
	return s
}

func VerifyCanary(cpu, pid int, containerID string) ([]Process, error) {
	return host.canary(cpu, pid, containerID)
}

// A daemon restart invalidates a run even if its replacement has the same mask.
func VerifyStable(before, after Snapshot) error {
	if !after.Passed {
		return fmt.Errorf("%s", after.Error)
	}
	if !before.Passed || before.CPU != after.CPU {
		return fmt.Errorf("placement profile changed during run")
	}
	if len(before.Processes) != len(after.Processes) {
		return fmt.Errorf("daemon process set changed during run")
	}
	for i, p := range before.Processes {
		q := after.Processes[i]
		if p.PID != q.PID || p.Role != q.Role || p.Cgroup != q.Cgroup || p.EffectiveCPUs != q.EffectiveCPUs || p.EffectiveMems != q.EffectiveMems {
			return fmt.Errorf("%s placement or process identity changed during run", p.Role)
		}
	}
	return nil
}

func (fs filesystem) canary(cpu, pid int, containerID string) ([]Process, error) {
	p, err := fs.process(pid, cpu, "probe", true)
	evidence := []Process{p}
	if err != nil {
		return evidence, err
	}
	if containerID == "" || p.Cgroup != "/dlbench.slice/docker-"+containerID+".scope" {
		return evidence, fmt.Errorf("probe cgroup %q does not match its expected Docker scope", p.Cgroup)
	}
	cmd, err := readTrim(filepath.Join(fs.proc, strconv.Itoa(p.ParentPID), "cmdline"))
	if err != nil {
		return evidence, err
	}
	if filepath.Base(strings.Split(cmd, "\x00")[0]) != "containerd-shim-runc-v2" {
		return evidence, fmt.Errorf("probe parent PID %d is not containerd-shim-runc-v2", p.ParentPID)
	}
	shim, err := fs.process(p.ParentPID, cpu, "shim", true)
	evidence = append(evidence, shim)
	return evidence, err
}
