# Release Binary

This directory can hold the prebuilt Linux amd64 binary used by low-resource
Docker builds.

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
