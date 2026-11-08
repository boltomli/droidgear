# Shared helper for the two measurement scripts: find the app's real top-level window.
#
# `Process.MainWindowHandle` is not it. Rune keeps a small hidden helper window, and .NET's "main
# window" heuristic can pick that one, which made an earlier version of these scripts time — and
# screenshot — a 15x15 window at the top-left of the screen. This enumerates the process's own
# top-level windows and takes the largest visible one, which is the window a user sees.

Add-Type @"
using System;
using System.Runtime.InteropServices;
public static class DgWin {
    [StructLayout(LayoutKind.Sequential)] public struct RECT { public int Left, Top, Right, Bottom; }
    public delegate bool EnumProc(IntPtr hWnd, IntPtr lParam);
    [DllImport("user32.dll")] static extern bool EnumWindows(EnumProc cb, IntPtr lParam);
    [DllImport("user32.dll")] static extern uint GetWindowThreadProcessId(IntPtr hWnd, out uint pid);
    [DllImport("user32.dll")] static extern bool IsWindowVisible(IntPtr hWnd);
    [DllImport("user32.dll")] static extern bool GetWindowRect(IntPtr hWnd, out RECT rect);
    [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr hWnd);
    [DllImport("user32.dll")] public static extern bool BringWindowToTop(IntPtr hWnd);
    [DllImport("user32.dll")] public static extern bool ShowWindow(IntPtr hWnd, int cmd);
    [DllImport("user32.dll")] public static extern void SwitchToThisWindow(IntPtr hWnd, bool alt);
    [DllImport("user32.dll", EntryPoint = "PrintWindow")] static extern bool PrintWindowFull(IntPtr hWnd, IntPtr hdc, uint flags);
    [DllImport("user32.dll")] public static extern IntPtr GetForegroundWindow();
    [DllImport("user32.dll")] static extern bool SetProcessDpiAwarenessContext(IntPtr ctx);

    public const long MinArea = 10000; // px^2: skips 15x15 helper windows

    /// Physical pixels for `GetWindowRect` and `CopyFromScreen`: without this the caller's
    /// coordinates are DPI-virtualised and the two disagree (a window reported at 101 is at 152).
    public static void DpiAware() {
        try { SetProcessDpiAwarenessContext(new IntPtr(-4)); } catch { }
    }

    /// Brings `h` to the front so a screen capture sees it: `SetForegroundWindow` alone can be
    /// refused for a background process, and `ShowWindow(SW_SHOW)` re-asserts the window.
    public static void Raise(IntPtr h) {
        ShowWindow(h, 5); // SW_SHOW
        BringWindowToTop(h);
        SetForegroundWindow(h);
        // The one that gets past the foreground lock for a background caller.
        SwitchToThisWindow(h, true);
    }

    /// Whether the window is the foreground window now (so a screen capture shows it).
    public static bool IsForeground(IntPtr h) { return GetForegroundWindow() == h; }

    /// Asks the window to draw itself into `hdc` (PW_RENDERFULLCONTENT), which works while other
    /// windows cover it.
    public static bool PrintWindow(IntPtr h, IntPtr hdc) { return PrintWindowFull(h, hdc, 2); }

    /// The largest visible window owned by `pid`, or IntPtr.Zero while there is none.
    public static IntPtr Biggest(uint pid, out RECT best) {
        IntPtr found = IntPtr.Zero;
        RECT bestRect = new RECT();
        long bestArea = 0;
        EnumWindows(delegate(IntPtr h, IntPtr l) {
            uint owner;
            GetWindowThreadProcessId(h, out owner);
            if (owner != pid || !IsWindowVisible(h)) { return true; }
            RECT r;
            if (!GetWindowRect(h, out r)) { return true; }
            long area = (long)(r.Right - r.Left) * (r.Bottom - r.Top);
            if (area >= MinArea && area > bestArea) { bestArea = area; found = h; bestRect = r; }
            return true;
        }, IntPtr.Zero);
        best = bestRect;
        return found;
    }
}
"@

# Before anything reads a window rect: physical pixels everywhere.
[DgWin]::DpiAware()

# Waits up to $TimeoutSeconds for the app's real window; returns the handle (or IntPtr.Zero), with
# the time it took in $script:WindowWaitMs.
function Wait-RealWindow($proc, [double]$TimeoutSeconds = 20) {
    $watch = [System.Diagnostics.Stopwatch]::StartNew()
    while ($watch.Elapsed.TotalSeconds -lt $TimeoutSeconds) {
        $proc.Refresh()
        if ($proc.HasExited) { break }
        $rect = New-Object DgWin+RECT
        $handle = [DgWin]::Biggest([uint32]$proc.Id, [ref]$rect)
        if ($handle -ne [IntPtr]::Zero) {
            $script:WindowWaitMs = $watch.Elapsed.TotalMilliseconds
            $script:WindowRect = $rect
            return $handle
        }
        Start-Sleep -Milliseconds 4
    }
    $script:WindowWaitMs = $null
    $script:WindowRect = New-Object DgWin+RECT
    return [IntPtr]::Zero
}
