# Builds Linux amd64 release binaries into ./release by using Docker on the
# current machine. The target VM can then build Dockerfile.prebuilt without
# compiling Go code.

$ErrorActionPreference = "Stop"

if (-not $env:VERSION) {
    $env:VERSION = (git describe --tags --always --dirty)
}
if (-not $env:COMMIT) {
    $env:COMMIT = (git rev-parse --short HEAD)
}
if (-not $env:BUILD_DATE) {
    $env:BUILD_DATE = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
}

New-Item -ItemType Directory -Force -Path "release" | Out-Null

docker build `
    -f Dockerfile.binary `
    --target export `
    --output type=local,dest=release `
    --build-arg GO_VERSION=1.26.4 `
    --build-arg VERSION="$env:VERSION" `
    --build-arg COMMIT="$env:COMMIT" `
    --build-arg BUILD_DATE="$env:BUILD_DATE" `
    --build-arg TARGETOS=linux `
    --build-arg TARGETARCH=amd64 `
    --build-arg CGO_ENABLED=1 `
    --build-arg OUTPUT_SUFFIX= `
    .

docker build `
    -f Dockerfile.binary `
    --target export `
    --output type=local,dest=release `
    --build-arg GO_VERSION=1.26.4 `
    --build-arg VERSION="$env:VERSION" `
    --build-arg COMMIT="$env:COMMIT" `
    --build-arg BUILD_DATE="$env:BUILD_DATE" `
    --build-arg TARGETOS=linux `
    --build-arg TARGETARCH=amd64 `
    --build-arg CGO_ENABLED=0 `
    --build-arg OUTPUT_SUFFIX=-no-plugin `
    .

Write-Host "Built release/cli-proxy-api-linux-amd64"
Write-Host "Built release/cli-proxy-api-linux-amd64-no-plugin"
