#!/usr/bin/env bash
set -euo pipefail
repo=/home/higon/yifan/docker-lifecycle-bench
experiment="$repo/dist/newton-single-cpu-20261022"
cd "$repo"
export DOCKER_HOST=unix:///var/run/docker.sock
unset DOCKER_CONTEXT DOCKER_API_VERSION DOCKER_TLS_VERIFY DOCKER_CERT_PATH
test "$(git rev-parse HEAD)" = d740ea0811a39683341aa603a74eb3414c2f400d
test -z "$(git status --porcelain --untracked-files=no)"
test ! -e "$experiment/host-state.json"
test ! -e "$experiment/results"
sudo -n true
"$experiment/dlbench" version > "$experiment/version.txt"
sha256sum "$experiment/dlbench" scripts/single-cpu-host.py scripts/verify-result.py > "$experiment/artifacts.sha256"
"$experiment/placement.test" -test.v > "$experiment/placement-tests.log" 2>&1
"$experiment/dockerengine.test" -test.v > "$experiment/dockerengine-tests.log" 2>&1
PYTHONPATH=python/src python3 -m unittest discover -s python/tests -v > "$experiment/python-tests.log" 2>&1
"$experiment/dlbench" validate --spec "$experiment/campaign.yaml" --single-cpu 16 > "$experiment/validated.json"
"$experiment/dlbench" doctor > "$experiment/doctor.json"
docker ps -a --no-trunc --format '{{.ID}} {{.State}} {{.Names}}' | sort > "$experiment/containers.before.txt"
test -z "$(docker ps -q)"
test -z "$(sudo -n ctr --namespace moby tasks list -q)"
systemctl show docker.service containerd.service dlbench.slice --property=Id,MainPID,ActiveState,ControlGroup,AllowedCPUs,EffectiveCPUs > "$experiment/services.before.txt"
lscpu > "$experiment/lscpu.txt"
lscpu -e > "$experiment/cpu-topology.txt"
{
  date --iso-8601=seconds
  uname -a
  systemctl --version
  uptime
  df -h "$repo" /data/docker
  for item in thread_siblings_list physical_package_id core_id; do
    printf '%s: ' "$item"
    cat "/sys/devices/system/cpu/cpu16/topology/$item"
  done
  for item in scaling_driver scaling_governor scaling_cur_freq scaling_min_freq scaling_max_freq scaling_setspeed; do
    printf '%s: ' "$item"
    cat "/sys/devices/system/cpu/cpu16/cpufreq/$item" || true
  done
  printf 'SMT active: '
  cat /sys/devices/system/cpu/smt/active
} > "$experiment/host.before.txt"
docker image inspect sha256:53c0b9d5ec8d5aed43fee5b15258370c25de43e4764050b7f57e144ae713f499 > "$experiment/probe-image.json"
monitor_pid=
finish() {
  run_code=$?
  trap - EXIT INT TERM
  set +e
  if [ -n "$monitor_pid" ]; then
    kill -TERM "$monitor_pid"
    wait "$monitor_pid"
  fi
  restore_code=0
  if [ -f "$experiment/host-state.json" ]; then
    sudo -n python3 scripts/single-cpu-host.py restore --state "$experiment/host-state.json" > "$experiment/restore.log" 2>&1
    restore_code=$?
  fi
  systemctl show docker.service containerd.service dlbench.slice --property=Id,MainPID,ActiveState,ControlGroup,AllowedCPUs,EffectiveCPUs > "$experiment/services.after.txt"
  docker ps -a --no-trunc --format '{{.ID}} {{.State}} {{.Names}}' | sort > "$experiment/containers.after.txt"
  diff -u "$experiment/containers.before.txt" "$experiment/containers.after.txt" > "$experiment/containers.diff"
  containers_code=$?
  printf 'benchmark_exit=%s\nrestore_exit=%s\ncontainers_comparison_exit=%s\n' "$run_code" "$restore_code" "$containers_code" > "$experiment/execution-status.txt"
  date --iso-8601=seconds >> "$experiment/execution-status.txt"
  if [ "$restore_code" -ne 0 ] || [ "$containers_code" -ne 0 ]; then
    exit 90
  fi
  exit "$run_code"
}
trap finish EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
sudo -n python3 scripts/single-cpu-host.py prepare --cpu 16 --state "$experiment/host-state.json" > "$experiment/prepare.log" 2>&1
taskset -c 0 mpstat -P 16,272 1 > "$experiment/mpstat.log" &
monitor_pid=$!
"$experiment/dlbench" run --spec "$experiment/campaign.yaml" --single-cpu 16 > "$experiment/benchmark.stdout" 2> "$experiment/benchmark.stderr"
