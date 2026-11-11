# The window that never opened: WebView2 startup failures

One build of the app exited with status 1 before the window appeared. No source
under `main.go` had changed, the same tree had run minutes earlier, and every
build after that failed the same way. This records what the WebView2 error
actually meant, how it was found, and the one-line fix.

## Symptom

```
INF AssetServer Info: middleware=true handler=true devServerURL=http://localhost:9245
INF Connected to frontend dev server!
[WebView2] Environment created successfully
[WebView2 Error] error creating controller with 800700aa: The requested resource is in use.
```

The `WebView2 Error` line comes from `Chromium.errorCallback` in
`internal/webview2/pkg/edge/chromium.go`, which calls `os.Exit(1)`. So
`task run` / `wails3 dev` reported only `exit status 1`, and the window never
appeared.

Two codes turned up on the same machine, for the same cause:

| Code | Text | When |
|---|---|---|
| `800700aa` | ERROR_BUSY, "The requested resource is in use" | a previous profile folder was still there |
| `8000ffff` | E_UNEXPECTED, "Catastrophic failure" | after that profile had been moved aside and a new one could not be created |

`Environment created successfully` is printed first in both cases: the runtime
loads, the environment object is returned, and only the controller — the part
that needs a profile on disk — fails.

## Where the profile goes

`main.go` sets no `Windows.WebviewUserDataPath`, so the path is the loader's
default: `%APPDATA%\<exe name>` (`%APPDATA%\spriteware.exe`, not
`…\spriteware.exe.WebView2`). That is a **Medium integrity** location.

## The cause

The repository folder carried a **Low mandatory integrity label**, set with
inherit-child flags, plus two unusual ACEs:

```
<repo>  Everyone:(CI)(DENY)(DC)
        S-1-4-xxxxxxxx-xxxxxxxx:(OI)(CI)(W,D,DC)
        NT AUTHORITY\SYSTEM:(I)(OI)(CI)(F)
        ...
        Mandatory Label\Low Mandatory Level:(OI)(CI)(NW)
```

Windows derives a process's integrity level from its **executable file's**
label, and `(OI)(CI)` on the label meant every file created in the tree
inherited Low — including `bin\spriteware.exe`, freshly linked a second earlier.

A Low integrity process is not allowed to write up to Medium, so
`%APPDATA%\spriteware.exe` could not be opened or created. WebView2 reports that
as a controller failure, not a permissions error, which is why the message names
a resource nobody asked for.

The label is inherited, so this did not only affect one build: every binary built
in that tree was Low from then on.

## How it was found

The misleading part is that copying the exe out of the tree makes the problem
disappear, because the copy takes the destination folder's Medium label.

| Test | Result |
|---|---|
| `bin\spriteware.exe` at its build path | fails, every run |
| the same bytes copied to `%TEMP%` | starts, every run |
| the same bytes at the repo root, or one directory deeper | fails |
| the same bytes elsewhere on the same drive | starts |
| another WebView2 app on the machine (same runtime version) | working |
| `msedge.exe --headless=new --dump-dom about:blank` | exit 0 |

That last pair rules out the runtime, the machine and the GPU. The difference
was never the binary; it was the folder it was launched from.

The confirmation is one command:

```powershell
icacls bin\spriteware.exe | Select-String "Mandatory Label"
#   Mandatory Label\Low Mandatory Level:(I)(NW)      <- the fault
```

## The fix

Restore the label on the tree, recursively. `bin` keeps a stored copy of the
inherited label, so fixing only the exe regresses on the next build:

```powershell
icacls <repo> /setintegritylevel "(OI)(CI)M" /T /C
```

Verify both the folder and a fresh build:

```powershell
icacls <repo> | Select-String "Mandatory Label"
#   Mandatory Label\Medium Mandatory Level:(OI)(CI)(NW)
go build -buildvcs=false -gcflags=all=-l -o bin\spriteware.exe .
icacls bin\spriteware.exe | Select-String "Mandatory Label"
#   Mandatory Label\Medium Mandatory Level:(I)(NW)
```

## What it was not

Each of these was tested and cleared, and each is a plausible first guess:

- **`-gcflags=all="-l"` / `CGO_ENABLED=0`**, the dev values from
  `build/windows/Taskfile.yml`. Built each way separately and together: all fine
  once the label was Medium.
- **The WebView2 runtime.** A different WebView2 app was running on the same
  `157.0.4341.42` install while ours failed.
- **The GPU process.** `WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS=--disable-gpu`
  changed nothing.
- **A locked profile.** Moving `%APPDATA%\spriteware.exe` aside changed the code
  from `800700aa` to `8000ffff` and nothing else.
- **The app's own code.** `git status` was clean, the failing binary reported the
  repo's own revision, and a minimal Wails window built from the same module ran
  fine from outside the tree.

It was also not malware: Defender was on with real-time, behaviour monitoring and
tamper protection, with no recorded detections, and the condition is metadata on
a folder rather than anything that runs. The `S-1-4-…` SID is the non-unique
authority range that sandboxed restricted tokens use, so the combination — low
integrity, "Everyone cannot delete", write/delete granted to a restricted SID —
reads as the footprint of a tool that sandboxes its working directory. It was not
set from PowerShell: script-block logging (event 4104) covered the window and
held no `icacls`, `Set-Acl` or `setintegritylevel`.

## If it happens again

1. `icacls <exe>` first, before touching build flags or the runtime. `Low
   Mandatory Level` is the whole answer.
2. `800700aa` and `8000ffff` from `Chromium.errorCallback` mean "no profile on
   disk", not "another instance is running". Closing stray processes will not
   help.
3. Note what ran against the folder just before it started, especially anything
   that sandboxes or snapshots a working directory. That names the tool that set
   the label; the ACLs alone cannot.
4. Test from outside the tree only to confirm the diagnosis, never as a
   workaround: a copy in `%TEMP%` hides the fault and leaves the next build
   broken.
