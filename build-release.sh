#!/usr/bin/env bash
#
# Builds the low-resource Linux amd64 release binary into ./release by using
# Docker on the current machine. The target VM can then build Dockerfile.prebuilt
# without compiling Go code.

set -euo pipefail

VERSION="${VERSION:-$(git describe --tags --always --dirty)}"
COMMIT="${COMMIT:-$(git rev-parse --short HEAD)}"
BUILD_DATE="${BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"

mkdir -p release

docker build \
  -f Dockerfile.binary \
  --target export \
  --output type=local,dest=release \
  --build-arg VERSION="${VERSION}" \
  --build-arg COMMIT="${COMMIT}" \
  --build-arg BUILD_DATE="${BUILD_DATE}" \
  --build-arg TARGETOS=linux \
  --build-arg TARGETARCH=amd64 \
  .

printf 'Built release/cli-proxy-api-linux-amd64-no-plugin\n'
