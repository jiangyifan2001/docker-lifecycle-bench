# Single logical CPU diagnostic runs

`--single-cpu N` runs serial sampling and all concurrent workers on the same
logical CPU. It is an opt-in diagnostic profile, not a Local-NUMA headline or a
maximum-machine-throughput result. The default campaign remains unchanged.

## What the option does

- Re-executes the harness from a pinned Linux thread, so its Go runtime threads
  inherit the mask, and sets `GOMAXPROCS=1`. Workers remain concurrent goroutines.
- Requires effective singleton cgroups for every local `dockerd` and `containerd`
  process and the prepared `dlbench.slice`. Checks every readable daemon thread.
- Requests `cpusetCpus=N`, `cgroupParent=dlbench.slice`, and runtime `runc` for
  every created container. Does not override an explicit conflicting YAML value.
- Starts an untimed, run-owned canary and checks its actual host PID, effective
  cpuset, thread masks, Docker scope, and `containerd-shim-runc-v2` parent. Removes
  it before warm-up; it contributes no lifecycle sample or throughput count.
- Saves `placement.before.json` and `placement.after.json` in the checksummed
  Result Bundle. A changed daemon PID, cgroup, CPU or memory set invalidates the
  result. Preparation/verification never runs inside a measured lifecycle.

Short-lived runc processes are constrained through cgroup inheritance and the
container parent, not individually traced. Before/after checks do not detect a
temporary administrative change that is reverted between checks. Do not change
placement or restart daemons during a campaign.

This is affinity, not exclusive CPU reservation. Other host work, IRQs, storage,
and kernel work can still affect the run. A logical CPU is not necessarily an
entire physical core. NUMA memory policy is **not** changed: `cpusetMems` remains
an independent setting and effective memory-node sets are recorded.

## Supported environment

Linux, rootful Docker, cgroup v2 with the systemd driver, the default runc runtime,
and a direct local Docker Unix socket are required. Standard
`docker.socket -> docker.service` socket activation is supported. Remote/proxied
sockets, alternative runtimes, hidden/inaccessible procfs evidence, and unrelated
unpinned Docker/containerd daemons cause a closed failure, not a partial fallback.

## Prepare separately as administrator

Choose an online logical CPU after inspecting `lscpu -e` and current host load.
CPU 16 below is an example. Confirm the host is idle and that changing both
services will not affect another operator. The helper refuses running Docker
containers, but this does not detect every non-Docker containerd workload.

The separate helper writes recovery state **before** changing runtime-only
`AllowedCPUs`. It does not restart services, change users/groups, or install
anything. It never runs automatically from `dlbench`.

```sh
# On liuhui, keep the repository, state and results under /home/higon/yifan.
# Create a new state filename for each preparation, never reuse an old one.
sudo python3 scripts/single-cpu-host.py prepare --cpu 16 \
  --state /home/higon/yifan/single-cpu-state-UNIQUE.json
```

The administrator may need to enter their password for this explicit step.
Ordinary benchmark execution does not invoke sudo or ask for a password.

## Run and restore

```sh
./bin/dlbench validate --spec examples/hygon-7490h-smoke.yaml --single-cpu 16
./bin/dlbench run --spec examples/hygon-7490h-smoke.yaml --single-cpu 16

# After measurement and cleanup, even if the campaign failed:
sudo python3 scripts/single-cpu-host.py restore \
  --state /home/higon/yifan/single-cpu-state-UNIQUE.json
```

Restore reinstates the previously effective `AllowedCPUs` properties and stops
the helper-created slice if it was originally inactive and is now empty. It does
not rewrite persistent unit files. For systemd versions that clear the property
but leave the old kernel cpuset active, it also clears that unit's `cpuset.cpus`
to restore inheritance, and verifies `EffectiveCPUs` against the saved value.
If the kernel rejects clearing a populated cpuset (`ENOSPC`), it explicitly
restores the original effective CPU list and prints a compatibility warning.
That restores the available CPU budget but leaves a runtime explicit mask;
returning to an empty/inherited mask requires a later administrator-approved
service restart on that kernel. The helper never restarts services itself.
Keep the recovery JSON. If a run was forcibly
killed and left a container, inspect its ownership labels and remove only that
run's containers before restoring; never use a global Docker prune.

The equivalent YAML is `placement: {singleCpu: 16}`. Normalized input includes
the expanded container placement. CPU 0 is valid; negative IDs and conflicts
are invalid. The optional Python wrapper forwards `--single-cpu` unchanged.

## Acceptance checks

A successful single-CPU smoke should have zero sample errors, the requested
serial count and concurrent points, passed pre/post placement evidence, a probe
and shim in the same CPU envelope, no leftover run-owned containers, and matching
checksums. Independently recompute E2E mean/percentiles and throughput from raw
operations. Do not sum overlapping stage latencies to derive E2E.

One smoke run validates the executable path, not statistical convergence,
cross-platform comparability, CPU exclusivity, or the unimplemented full design.

Mechanism references: [systemd AllowedCPUs](https://www.freedesktop.org/software/systemd/man/latest/systemd.resource-control.html),
[Linux cgroup v2 cpuset](https://docs.kernel.org/admin-guide/cgroup-v2.html#cpuset),
and [Docker cgroup parent](https://docs.docker.com/reference/cli/dockerd/#default-cgroup-parent).
