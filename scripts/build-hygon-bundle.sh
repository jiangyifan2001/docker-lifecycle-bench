#!/bin/sh
set -eu

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
go_binary=${GO:-go}
version=${VERSION:-0.1.0-dev}
commit=$(git -C "$repo_dir" rev-parse --verify HEAD)
short_commit=$(printf '%s' "$commit" | cut -c1-12)
package_name="dlbench-${version}-linux-amd64-${short_commit}"
dist_root="$repo_dir/dist"
archive="$dist_root/$package_name.tar.gz"

if ! git -C "$repo_dir" diff --quiet || ! git -C "$repo_dir" diff --cached --quiet; then
  printf 'refusing to package a dirty working tree; commit the reviewed source first\n' >&2
  exit 1
fi

if ! command -v "$go_binary" >/dev/null 2>&1 && [ ! -x "$go_binary" ]; then
  printf 'Go executable not found: %s\n' "$go_binary" >&2
  exit 1
fi

if ! docker info >/dev/null 2>&1; then
  printf 'Docker/BuildKit is required to build the probe image; start Docker Desktop and retry\n' >&2
  exit 1
fi

mkdir -p "$dist_root"
if [ -e "$archive" ]; then
  printf 'refusing to overwrite existing archive: %s\n' "$archive" >&2
  exit 1
fi

stage=$(mktemp -d "$dist_root/.hygon-bundle.XXXXXX")
trap 'rm -rf "$stage"' EXIT HUP INT TERM
package_dir="$stage/$package_name"
mkdir -p "$package_dir"

GOOS=linux GOARCH=amd64 CGO_ENABLED=0 "$go_binary" build \
  -trimpath \
  -ldflags "-s -w -X main.version=$version -X main.commit=$commit" \
  -o "$package_dir/dlbench" \
  "$repo_dir/cmd/dlbench"

docker buildx build \
  --platform linux/amd64 \
  --provenance=false \
  --sbom=false \
  --tag dlbench-probe:smoke \
  --output "type=docker,dest=$package_dir/dlbench-probe-linux-amd64.tar" \
  "$repo_dir/probe"

cp "$repo_dir/examples/hygon-7490h-smoke.yaml" "$package_dir/campaign.yaml"
cp "$repo_dir/schema/campaign-v1alpha1.json" "$package_dir/campaign.schema.json"
cp "$repo_dir/docs/liuhui-runbook.md" "$package_dir/RUN.md"

{
  printf 'release_version=%s\n' "$version"
  printf 'source_commit=%s\n' "$commit"
  printf 'target=linux/amd64\n'
  printf 'probe_image=dlbench-probe:smoke\n'
} > "$package_dir/release-manifest.txt"

(
  cd "$package_dir"
  shasum -a 256 \
    dlbench \
    dlbench-probe-linux-amd64.tar \
    campaign.yaml \
    campaign.schema.json \
    RUN.md \
    release-manifest.txt > checksums.sha256
)

tar -czf "$archive" -C "$stage" "$package_name"
(
  cd "$dist_root"
  shasum -a 256 "$(basename "$archive")" > "$(basename "$archive").sha256"
)

printf 'transfer bundle: %s\n' "$archive"
printf 'archive checksum: %s.sha256\n' "$archive"
