# Docker Lifecycle Bench

`docker-lifecycle-bench` is a host-platform system microbenchmark for Docker
container lifecycle latency and throughput. The first implementation milestone
is deliberately narrow: execute one auditable diagnostic smoke campaign on a
bare-metal Hygon 7490H host.

The timed path has one implementation. A Go binary (`dlbench`) drives the local
Docker Engine API, timestamps lifecycle boundaries, receives readiness from a
minimal static C probe, and writes an immutable result bundle. A thin Python
wrapper may invoke the same binary, but it does not schedule load or measure
time.

```text
Python wrapper (optional)
        |
        v
Go dlbench on the host -- Docker Unix socket --> dockerd/containerd/runc
        ^                                             |
        |------------- probe READY frame <------------|
        |
        +--> Result Bundle
```

## Phase 0 scope

- Linux bare metal and rootful Docker
- native `linux/amd64` execution on the Hygon host
- instrumented `FullLifecycle` only:
  `create -> start -> ready -> SIGKILL -> wait -> remove`
- serial latency samples
- a small closed-loop capacity sweep
- diagnostic results only; no cross-platform ranking

`CreateRemove`, `ReusedStartExit`, offered-load experiments, formal NUMA
sensitivity, and publishable p99.9 statistics are intentionally deferred.

## Build

Go 1.24 or newer is required. Build the host binary on Linux or cross-compile it
from macOS:

```sh
make test
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o dist/linux-amd64/dlbench ./cmd/dlbench
docker buildx build --platform linux/amd64 --load -t dlbench-probe:smoke probe
```

The probe image may be built under emulation on the development Mac, but it
must run natively on the benchmark host. QEMU/emulation is never allowed during
a measured run.

## First Hygon smoke run

The benchmark operator must already have access to the local Docker socket.
`dlbench` never creates users, changes group membership, invokes `sudo`, installs
packages, prunes Docker globally, or changes host tuning.

```sh
dlbench doctor
dlbench validate --spec examples/hygon-7490h-smoke.yaml
dlbench run --spec examples/hygon-7490h-smoke.yaml
```

The result directory contains the normalized campaign, environment evidence,
one NDJSON record per attempted cycle, a summary, final run status, and SHA-256
checksums. Failed cycles remain in the raw result.

To create a single transfer bundle on the development Mac, start Docker
Desktop/BuildKit and run:

```sh
GO=/path/to/go VERSION=0.1.0-dev ./scripts/build-hygon-bundle.sh
```

The script cross-compiles `dlbench`, builds a `linux/amd64` Docker image archive,
copies the campaign and schema, writes checksums, and creates one
`dist/dlbench-<version>-linux-amd64-<commit>.tar.gz`. Follow the bundled
`RUN.md` on liuhui.

The historical design documents remain unchanged for backup and review. The
accepted decisions in `docs/adr/` are the current design record.
