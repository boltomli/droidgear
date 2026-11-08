# Rune GUI vs the Tauri build, measured

Same machine, same page (Droid → BYOK models, both reading the same `~/.factory/settings.json`),
4 timed runs each after a discarded warm-up, medians reported. Windows 11 at 150 % scaling, AMD
Ryzen 780M (integrated Radeon), release builds.

|                                   | Tauri 2 (`droidgear.exe`) | Rune (`droidgear-gui.exe`) |
| --------------------------------- | ------------------------- | -------------------------- |
| Window on screen                  | 635 ms                    | **506 ms**                 |
| UI on screen (first real paint)   | 880 ms                    | 980 ms                     |
| UI settled (last pixel change)    | 962 ms                    | ~1.5 s (see below)         |
| RSS after 6 s, whole process tree | 473 MB                    | **143 MB**                 |
| Idle CPU, whole tree              | 0.3 %                     | **0 %**                    |
| Processes / threads               | 7 / 27                    | **1** / 15                 |
| Executable                        | **13.3 MB**               | 25.6 MB                    |
| Draws with                        | WebView2 (Chromium)       | wgpu (DX12 here)           |

`473 MB` is the Tauri app plus the six `msedgewebview2.exe` processes it owns (33 MB in the host
binary, 440 MB in the webview tree). `143 MB` is one process plus the ~10 MB `conhost.exe` Windows
attaches when the app is started from a shell.

## Reading it

- **Memory is the win: 3.3× less, in one process.** A settings-and-Markdown app does not need a
  browser engine, and this is what one costs.
- **Idle CPU is zero.** The WebView keeps a renderer and a GPU process alive that sample at ~0.3 % of
  a core between frames; Rune's window draws nothing while nothing changes.
- **Startup is roughly at parity** — Rune's window is up ~130 ms sooner, its UI about 100 ms later.
  Both reach a usable page in under a second.
- **But Rune spends that first ~470 ms showing an empty window.** Tauri creates its window hidden,
  renders into it and shows it already painted, so nothing blank ever reaches the screen. Rune shows
  the window at native creation (506 ms) and paints at ~980 ms. That gap, not the total, is the
  thing to fix.
- The binary is 1.9× larger: it carries the renderer, Parley's text stack, the embedded fonts and
  ICU data, where a WebView app borrows them from the machine.

## Where the launch time goes

`DROIDGEAR_TRACE_STARTUP=1` prints the app-side milestones (milliseconds from entering `main`,
median of 3):

|                                                                                 |        |
| ------------------------------------------------------------------------------- | ------ |
| `App::init` done (preferences JSON, language negotiation, first page from core) | 33 ms  |
| first view build (window exists, GPU device and pipeline ready)                 | 977 ms |

So the app's own work is 33 ms; everything else is platform + GPU + first frame. The gap from
`main` to the first view build splits roughly into winit/platform startup, window creation, wgpu
device creation and the uber-pipeline compile; the window is on screen partway through it.

The ~500 ms between "window on screen" and "UI on screen" is the part worth attacking, and it is
inside Rune rather than this app: the window is created visible before the first frame exists, and
the first frame's pipeline/shader work happens after.

## What was tried

**`rune::warm_gpu` (build the device and renderer on a worker while the window is created)** — the
documented Rune launch pattern, and the first thing to try. Measured A/B with screen sampling,
4 runs each:

|                 | UI on screen              |
| --------------- | ------------------------- |
| without warm-up | 861–1025 ms (median ~877) |
| with warm-up    | 861–917 ms (median ~874)  |

No measurable difference, so it was **removed** again rather than kept on faith: on this machine the
device is not what the launch waits for. (It may matter on a slower driver, and it is the right
pattern on macOS, where `prewarm_gpu` is the framework's own path.)

**Deferring the app's own startup work** is not worth it yet: `App::init` is 33 ms, and the pages
that will get expensive (sessions, channels) do not load anything at launch.

## Next steps, in order

1. **Do not show an empty window.** Either set the window's background to the page colour before the
   first frame (needs a `WindowOptions`/`Chrome` knob from Rune), or create the window hidden and
   show it in `on_ready` — Tauri's approach, which trades "window 130 ms later" for "never blank".
2. **Precompile the uber shader on Windows.** On macOS Rune ships a prebuilt `.metallib` for exactly
   this (`Renderer::new` 232 ms → 85 ms). On Windows the HLSL→DXBC compile through FXC happens at
   first paint, per process. Worth an upstream issue with these numbers.
3. **Then re-measure** with `measure-paint.ps1`; this repository has the scripts and the baseline.

## Method

```powershell
# window on screen, memory and idle CPU of the whole process tree
powershell -File rune-gui/scripts/measure-startup.ps1 -Exe src-tauri/target/release/droidgear.exe

# when the UI actually arrives: the screen, sampled every ~25 ms
powershell -File rune-gui/scripts/measure-paint.ps1 -Exe src-tauri/target/release/droidgear.exe
```

Both scripts wait for the app's **largest visible top-level window**, enumerated per process id, and
`measure-paint.ps1` raises it with `SwitchToThisWindow` (which gets past the foreground lock a
background caller hits) before reading the screen. Three traps, each of which gave wrong numbers
before the scripts handled it:

1. `Process.MainWindowHandle` is not the app window. Rune keeps a 15×15 hidden helper window and
   .NET's heuristic can pick that one, which made an earlier version of these numbers time — and
   screenshot — a 15×15 window at the top-left of the screen.
2. `CopyFromScreen` needs physical pixels. Without `SetProcessDpiAwarenessContext`, window rects are
   DPI-virtualised (a window reported at 101 is at 152) and the capture lands somewhere else.
3. A screen capture only shows what is on top, and a window's own pixels are the reliable signal.
   `-Method print` uses `PrintWindow(PW_RENDERFULLCONTENT)` so the window draws itself even while
   covered — but it returns the last composited frame, which on this machine ran ~450 ms behind the
   screen for the wgpu window. `-Method screen` (the default) is what the numbers above use.

Caveats worth stating: one machine, one integrated GPU, a warm file cache, one page, and a dark UI on
a dark window — the threshold for "a pixel changed" has to be low enough to catch text appearing on a
near-black background, which is why `-ChangeThreshold 0.005` is worth passing when measuring a dark
theme. The Rune figures are for the spike in this directory, not for a finished app.
