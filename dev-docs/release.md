# Releasing

The git tag is the only place a version is written down. Everything else is
stamped from it.

## Cut a release

```sh
git tag v1.2.3
git push origin v1.2.3
```

`.github/workflows/release.yml` then:

1. refuses anything that is not `vMAJOR.MINOR.PATCH`;
2. runs `go vet ./...` and `go test ./...` on a machine with the GTK4 deps, so
   the window (which embeds `frontend/dist`) actually compiles;
3. builds Windows, macOS and Linux in parallel;
4. publishes a GitHub Release with all artifacts and `SHA256SUMS.txt`.

If any build fails, `cleanup-tag-on-failure` deletes the tag, so the same
version can be pushed again after the fix.

## What the tag stamps

`go run ./cmd/stamp v1.2.3` writes the version into:

| File | What reads it |
|---|---|
| `build/config.yml` | Wails project identity, MSIX |
| `build/windows/info.json` | the exe's version resource (via `generate syso`) |
| `build/windows/wails.exe.manifest` | assembly identity (four-part) |
| `build/windows/nsis/wails_tools.nsh` | the installer's `VIProductVersion` |
| `build/linux/nfpm/nfpm.yaml` | the `.deb` |
| `build/darwin/Info.plist` | `CFBundleShortVersionString` / `CFBundleVersion` |

The binary itself gets `-ldflags "-X github.com/d3uceY/spriteware/backend.Version=1.2.3"`,
which the platform Taskfiles add whenever `VERSION` is set. A local build without
`VERSION` keeps the fallback in `backend/mcp.go`.

The stamp tool fails when a marker is missing, so a renamed key breaks the
release instead of shipping a stale number.

## Artifacts

| Platform | File |
|---|---|
| Windows installer | `SpriteWare-windows-amd64-installer.exe` |
| Windows portable | `SpriteWare-windows-amd64.exe` |
| macOS (Apple Silicon + Intel) | `SpriteWare-macos-universal.dmg` |
| Linux binary | `SpriteWare-linux-amd64` |
| Linux package | `SpriteWare-linux-amd64.deb` |

## Version and updates inside the app

The version badge in the top strip is `backend.Version`, read through
`AppService.GetVersion`. On startup the window also calls
`AppService.CheckForUpdate`, which compares that string with the newest published
GitHub release. A newer release opens a dialog whose Download button goes to the
README's [download table](../README.md#download), so one link serves every
platform. The app never downloads or installs anything itself.

## Reproduce a release build locally

```powershell
go run ./cmd/stamp v1.2.3
wails3 task windows:create:nsis:installer ARCH=amd64 VERSION=1.2.3
go run ./cmd/stamp 0.1.0     # put the working tree back
```

`bin\spriteware.exe` and `bin\spriteware-amd64-installer.exe` are then the same
binaries the workflow uploads.
