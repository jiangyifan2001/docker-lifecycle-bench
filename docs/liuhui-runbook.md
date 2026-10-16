# Run the Phase 0 bundle on liuhui

This bundle is a diagnostic integration run, not a comparable benchmark
result. Run it from an otherwise idle bare-metal host using a trusted operator
that already has access to the rootful Docker socket.

## 1. Verify the transferred archive

On liuhui, place the `.tar.gz` file and its `.sha256` file in the same empty
directory:

```sh
sha256sum -c dlbench-*.tar.gz.sha256
tar -xzf dlbench-*.tar.gz
cd dlbench-*-linux-amd64-*
sha256sum -c checksums.sha256
```

Stop if either checksum command fails.

## 2. Load and verify the native probe image

```sh
docker load --input dlbench-probe-linux-amd64.tar
docker image inspect dlbench-probe:smoke \
  --format '{{.Os}}/{{.Architecture}} {{.Id}}'
```

The first field must be `linux/amd64`. Do not continue if Docker reports an
emulation warning or a different architecture.

## 3. Qualify the host

```sh
chmod +x dlbench
./dlbench doctor > doctor.json
```

`doctor` must exit with status 0. Inspect `doctor.json`; required checks must
pass. In particular, the host must be Linux `amd64`, bare metal, cgroup v2, and
connected to rootful Docker.

## 4. Validate and run the campaign

```sh
./dlbench validate --spec campaign.yaml > campaign.preview.json
./dlbench run --spec campaign.yaml
```

The command prints the new Result Bundle path to stderr. The default location
is `results/<run-id>/`. Copy that whole directory back for analysis; do not copy
only `summary.json`.

## 5. Confirm completion

```sh
result_dir=$(find results -mindepth 1 -maxdepth 1 -type d | sort | tail -1)
cat "$result_dir/status.json"
sha256sum -c "$result_dir/checksums.sha256"
```

The expected status is `SUCCESS`. Any `BLOCKED`, `RUN_FAILED`, `INVALID_RESULT`,
or `INTERRUPTED` result must be preserved and investigated instead of rerun
silently.

## Source-bundle alternative

For the first diagnostic smoke only, the source archive may be built natively
on liuhui instead of loading a prebuilt probe archive:

```sh
chmod +x scripts/build-on-liuhui.sh
./scripts/build-on-liuhui.sh
```

This requires Go 1.24 or newer and access to the Docker daemon. The script does
not install prerequisites. After it completes, allow CPU temperature,
frequency and host load to return to idle before running
`dist/liuhui-native/dlbench`. A source-built smoke remains a Diagnostic Result.
