# Phase 0: Hygon 7490H smoke campaign

Phase 0 has one success criterion: execute the complete instrumented
`FullLifecycle` path natively on the Hygon 7490H bare-metal host and preserve an
auditable diagnostic Result Bundle. It is not a publishable platform result.

## Components

| Component | Location | Responsibility |
|---|---|---|
| `dlbench` | benchmark host | Docker API operations, raw monotonic timestamps, load generation, validation, result writing |
| static C probe | container PID 1 | emit one binary READY frame and block until killed |
| Python client | operator environment | invoke `dlbench`; never time or schedule lifecycle operations |
| Docker Engine stack | benchmark host | execute the lifecycle under test |

The host path is:

```text
dlbench -> Docker Unix socket -> dockerd -> containerd -> runc/shim -> probe
   ^                                                               |
   +--------------------- READY frame ------------------------------+
```

## Required host evidence

`dlbench doctor` records and checks:

- Linux `amd64` execution
- bare-metal status reported by `systemd-detect-virt`
- `lscpu` and `numactl --hardware` output
- cgroup v2
- access to rootful Docker
- Docker, kernel, cgroup and storage-driver versions

CPU and memory pinning may be empty for this first smoke campaign. If empty,
the run records topology but makes no Local-NUMA claim.

## Run sequence

1. Load the native `linux/amd64` probe image and verify its digest.
2. Run `dlbench doctor`; do not bypass a required failure.
3. Validate and normalize the campaign YAML.
4. Warm up until both 100 cycles and 30 seconds have elapsed.
5. Record 1,000 serial lifecycle samples.
6. Run one 30-second closed-loop point at each of 1, 2, 4 and 8 workers.
7. Finalize status, summary and SHA-256 checksums even when the run fails.

Each successful cycle follows:

```text
create -> pre-attach stdout -> start -> READY -> SIGKILL -> wait -> remove
```

The summary reports p50, p95 and p99. It deliberately omits p99.9. Failed
operations remain in `operations.ndjson`, contribute to the error rate, and do
not enter successful latency percentiles.

## Safety boundary

The runner creates containers with `io.dlbench.managed=true` and a unique run
label. Error cleanup verifies both labels before forced removal. It never runs
global prune, changes host users/groups, calls `sudo`, installs packages, or
modifies Docker daemon configuration.
