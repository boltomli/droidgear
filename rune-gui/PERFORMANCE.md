# Rune GUI vs the Tauri build, measured

Same machine, same page (Droid → BYOK models, both reading the same `~/.factory/settings.json`),
5 timed runs each after a discarded warm-up, medians reported. Windows 11, AMD Ryzen 780M (integrated
Radeon), release builds.

|                                     | Tauri 2 (`droidgear.exe`)     | Rune (`droidgear-gui.exe`) |
| ----------------------------------- | ----------------------------- | -------------------------- |
| Window visible                      | **33 ms** (a loader is shown) | 445 ms                     |
| UI settled (pixels still for 500ms) | **1.45 s**                    | 1.99 s                     |
| RSS after 6 s, whole process tree   | 473 MB                        | **137 MB**                 |
| Idle CPU, whole tree                | 0.83 %                        | **0 %**                    |
| Processes / threads                 | 7 / 27                        | **1** / 15                 |
| Executable                          | **13.3 MB**                   | 25.6 MB                    |
| Where the UI is drawn               | WebView2 (Chromium)           | wgpu (DX12 here)           |

`473 MB` is the Tauri app plus the six `msedgewebview2.exe` processes it owns (33 MB in the host
binary, 441 MB in the webview tree). `137 MB` is one process plus the 10 MB `conhost.exe` that
Windows attaches when the app is started from a shell; the app itself settles at ~129 MB.

## What the numbers mean

- **Memory is the real win: 3.5× less, in one process.** A settings-and-Markdown app does not need a
  browser engine, and this is the cost of one.
- **Idle CPU goes to zero.** The WebView keeps a renderer and a GPU process alive that sample at
  ~1 % of a core between frames; Rune's window draws nothing while nothing changes.
- **Startup is currently _slower_, not faster** — and it is not the framework's ceiling: the 1.99 s
  to a settled UI is dominated by wgpu device creation, which Rune's own headless logs report as
  **1.0–2.4 s** on this integrated GPU (first frame after that is 43–103 ms). Tauri wins this round
  by showing a loader at 33 ms while Chromium comes up, then painting the UI at ~1.45 s.
- The binary is 1.9× larger because it carries the renderer, Parley's text stack, the embedded fonts
  and ICU data; a WebView app borrows those from the machine.

## Method

```powershell
# time to a visible window, memory and idle CPU of the whole process tree
powershell -File rune-gui/scripts/measure-startup.ps1 -Exe src-tauri/target/release/droidgear.exe

# when the real UI arrives: 25 ms screen samples of the window, first change and last change
powershell -File rune-gui/scripts/measure-paint.ps1 -Exe src-tauri/target/release/droidgear.exe
```

Both scripts start the executable themselves, wait for `IsWindowVisible` (not just a window handle —
Tauri creates its window hidden and shows it from React), then sample. RSS and CPU are summed over
the process _and all descendants_, so a WebView's helpers are charged to the app that spawned them.
The paint script reads the screen, so the desktop must be clear; an overlapping window counts as a
change.

Caveats worth stating: one machine, one integrated GPU, a warm file cache, and only one page
(someone who opens a session with a long transcript, or the terminal, will see different numbers).
The Rune numbers are for the spike in this directory, not for a finished app.

## What to try next for startup

1. **Overlap device creation with the window.** Create the window, paint the page background from
   the CPU or a tiny first frame, and build the wgpu device on a worker so the 1–2 s of driver init
   does not sit between "window exists" and "UI is up". Rune's own "Launch fast" notes say slow work
   belongs in `on_ready`, after the first frame.
2. **Move the spike's own startup I/O off the first frame.** `App::init` currently reads
   preferences, models and specs synchronously; on a network home or a large config that adds to the
   same budget. `AppSpec::setup`/`poll` with a waker, or `rune::Latest`, is the intended shape.
3. **Check the backend cost.** DX12 on this iGPU may be the slow path; Vulkan, or a driver-specific
   option, is worth one measurement before blaming Rune.
4. **Measure a warm second window** (open a second window in the same process) to separate
   per-process driver init from per-frame cost.

For the record, the spike's own headless runs (no window, still a real GPU device) report
`gpu init 1.0–2.4 s, first frame 43–103 ms, 699 draw instances` for the models page at 1280×820,
which is the same device-init cost seen above.
