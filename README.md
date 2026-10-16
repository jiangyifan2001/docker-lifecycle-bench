# Docker Lifecycle Bench

`docker-lifecycle-bench` is a host-platform system microbenchmark for measuring
Docker container lifecycle latency and throughput. It is designed to compare
host platforms without treating any processor vendor, family, or model as the
definition of the benchmark.

The repository is currently at its Phase 0 milestone: one auditable diagnostic
implementation of a full container lifecycle, validated on a bare-metal
Linux/amd64 host. The included Hygon 7490H campaign is a first-host example,
not a restriction on the long-term processor scope.

## What it measures

The implemented `FullLifecycle` scenario measures one complete cycle:

```text
create -> attach stdout -> start -> probe ready -> SIGKILL -> wait -> remove
```

Each successful cycle records the following boundaries:

- container create acknowledgement;
- Docker start API acknowledgement;
- start-to-ready time reported by a minimal in-container probe;
- forced teardown and wait completion;
- remove API acknowledgement; and
- total acknowledged lifecycle time.

The harness runs both serial latency sampling and a closed-loop capacity sweep.
Capacity and latency remain separate result groups so that completed cycles per
second are not confused with an externally offered arrival rate.

## Architecture

The timed path has a single implementation:

```text
Python wrapper (optional)
        |
        v
Go dlbench on the host -- Docker Unix socket --> dockerd/containerd/runc
        ^                                             |
        |------------- binary READY frame <-----------|
        |
        +--> immutable Result Bundle
```

- **Go host harness (`dlbench`)**: owns scheduling, Docker API calls, timing,
  lifecycle cleanup, and result generation.
- **C probe**: runs as container PID 1, emits one fixed-size READY frame, and
  waits for teardown.
- **Python client**: optionally invokes the Go binary as a subprocess. It does
  not duplicate the timing engine or load generator.
- **Result Bundle**: preserves normalized input, environment evidence, raw
  operations, derived summaries, final status, and checksums.

The harness runs directly on the measured host. It is not run inside a
Docker-in-Docker container.

## Current scope

Phase 0 intentionally supports a narrow, diagnosable path:

- bare-metal Linux;
- rootful Docker with cgroup v2;
- native `linux/amd64` execution with no QEMU or ISA emulation;
- the instrumented `FullLifecycle` scenario;
- serial latency sampling;
- closed-loop capacity points; and
- diagnostic results only.

The design is processor-platform neutral. Support for additional native ISAs,
cloud environments, formal Local-NUMA and Remote-client NUMA profiles,
offered-load tests, additional lifecycle scenarios, and publishable p99.9
statistics is planned but is not implemented in Phase 0.

## Requirements

To build the project:

- Go 1.24 or newer;
- a C-capable Docker build environment for the probe image; and
- Python 3.10 or newer only if the optional wrapper is used.

To run a Phase 0 campaign:

- an otherwise idle bare-metal Linux host;
- a native `linux/amd64` `dlbench` binary and probe image;
- rootful Docker;
- cgroup v2; and
- a trusted operator with access to the local Docker Unix socket.

Membership in the `docker` group is effectively root-equivalent. Provision
Docker access as an explicit host-administration step; the benchmark does not
install packages, invoke `sudo`, create users, or modify group membership.

## Build and test

```sh
git clone https://github.com/jiangyifan2001/docker-lifecycle-test.git
cd docker-lifecycle-test

make test
make test-python
make build
make probe-image
```

The equivalent native Go build is:

```sh
go build -trimpath -o bin/dlbench ./cmd/dlbench
```

To cross-compile only the host harness from macOS:

```sh
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
  go build -trimpath -o dist/linux-amd64/dlbench ./cmd/dlbench
```

The probe image must execute natively on the benchmark host. Emulation is not
allowed in a measured run.

## Run the diagnostic campaign

Build or load the probe image before starting the campaign, then run:

```sh
./bin/dlbench doctor
./bin/dlbench validate --spec examples/hygon-7490h-smoke.yaml
./bin/dlbench run --spec examples/hygon-7490h-smoke.yaml
```

`doctor` checks the operating system, native architecture, bare-metal status,
cgroup version, Docker access, rootful mode, and available NUMA evidence. A
required failure blocks the run instead of silently weakening the experiment.

The example campaign leaves `cpusetCpus` and `cpusetMems` empty. It is therefore
a topology-recorded smoke test, not a Local-NUMA headline result.

## Campaign configuration

Campaigns use the versioned `dlbench.io/v1alpha1` YAML interface. The Phase 0
schema is available at [`schema/campaign-v1alpha1.json`](schema/campaign-v1alpha1.json).
The main controls are:

- probe image reference;
- output directory;
- container CPU and NUMA memory placement;
- minimum warm-up cycles and duration;
- serial measurement sample count and timeout; and
- capacity worker counts and duration per point.

Warm-up ends only after both its minimum cycle count and minimum duration have
been satisfied.

## Result Bundle

Every attempted run creates a new result directory, including blocked and
failed runs. A completed bundle contains:

```text
campaign.normalized.json  Expanded campaign input
environment.json          Host, Docker, cgroup, CPU, and NUMA evidence
manifest.json             Harness version and build identity
operations.ndjson         One raw record per attempted lifecycle
summary.json              Derived latency and capacity statistics
status.json               SUCCESS, BLOCKED, RUN_FAILED, or INTERRUPTED
checksums.sha256           Bundle integrity checksums
```

Preserve the whole directory. `summary.json` is derived data and does not
replace `operations.ndjson` or the environment evidence.

Smoke results are diagnostic. They must not be used for cross-platform ranking
unless the compared hosts satisfy the same frozen environment, workload,
placement, and artifact contract.

## Optional Python client

The Python package is intentionally thin:

```sh
python3 -m pip install -e ./python
dlbench-client --binary ./bin/dlbench validate \
  --spec examples/hygon-7490h-smoke.yaml
```

It delegates validation and execution to the canonical Go binary.

## Transfer to a benchmark host

With Docker Desktop or another BuildKit environment running on the development
machine, create a Linux/amd64 transfer bundle with:

```sh
GO=/path/to/go VERSION=0.1.0-dev ./scripts/build-hygon-bundle.sh
```

The generated archive contains the host binary, probe image, campaign,
schema, run instructions, and SHA-256 checksums. Follow
[`docs/liuhui-runbook.md`](docs/liuhui-runbook.md) for the current first-host
workflow.

## Design documentation

- [`CONTEXT.md`](CONTEXT.md) defines the benchmark's domain language.
- [`docs/adr/`](docs/adr/) records accepted architecture decisions.
- [`docker-lifecycle-test-plan.md`](docker-lifecycle-test-plan.md) and
  [`docker-lifecycle-test-setup.md`](docker-lifecycle-test-setup.md) are the
  preserved design documents that preceded the executable implementation.

Generated binaries, transfer archives, and Result Bundles belong under ignored
directories such as `dist/` or `results/`; they are not source-controlled by
default.
