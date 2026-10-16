#!/bin/sh
set -eu

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
go_binary=${GO:-go}
output_dir="$repo_dir/dist/liuhui-native"

if [ "$(uname -s)" != "Linux" ] || [ "$(uname -m)" != "x86_64" ]; then
  printf 'this script requires native Linux x86_64; got %s %s\n' "$(uname -s)" "$(uname -m)" >&2
  exit 1
fi

if ! command -v "$go_binary" >/dev/null 2>&1; then
  printf 'Go 1.24 or newer is required to build dlbench\n' >&2
  exit 1
fi

if ! docker info >/dev/null 2>&1; then
  printf 'the benchmark operator cannot reach the Docker daemon\n' >&2
  exit 1
fi

mkdir -p "$output_dir"

(
  cd "$repo_dir"
  "$go_binary" test ./...
  CGO_ENABLED=0 "$go_binary" build -trimpath -o "$output_dir/dlbench" ./cmd/dlbench
)

docker build --tag dlbench-probe:smoke "$repo_dir/probe"

image_platform=$(docker image inspect dlbench-probe:smoke --format '{{.Os}}/{{.Architecture}}')
if [ "$image_platform" != "linux/amd64" ]; then
  printf 'probe image platform is %s, want linux/amd64\n' "$image_platform" >&2
  exit 1
fi

cp "$repo_dir/examples/hygon-7490h-smoke.yaml" "$output_dir/campaign.yaml"
cp "$repo_dir/schema/campaign-v1alpha1.json" "$output_dir/campaign.schema.json"

(
  cd "$output_dir"
  sha256sum dlbench campaign.yaml campaign.schema.json > checksums.sha256
)

printf 'native artifacts written to %s\n' "$output_dir"
printf 'do not benchmark immediately after this build; allow the host to return to idle state first\n'

