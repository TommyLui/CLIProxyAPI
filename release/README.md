# Release Binaries

This directory can hold prebuilt Linux amd64 binaries used by low-resource
Docker builds.

- `cli-proxy-api-linux-amd64` supports dynamic library plugins.
- `cli-proxy-api-linux-amd64-no-plugin` is a portable static fallback without
  dynamic library plugin support.

Build the binary on a machine with enough resources:

```bash
./build-release.sh
```

Or on Windows PowerShell:

```powershell
.\build-release.ps1
```

Then build the lightweight runtime image on the target Linux amd64 VM:

```bash
docker build -f Dockerfile.prebuilt -t cli-proxy-api:prebuilt .
```
