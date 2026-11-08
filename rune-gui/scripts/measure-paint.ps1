# Compares how long an app takes to put its real UI on screen, by watching its pixels.
#
# `measure-startup.ps1` times the window becoming visible, which flatters a WebView app that shows a
# loader first and paints the UI later. This script samples what is actually on screen every ~25 ms
# and reports:
#
#   visible     - the app's real window appeared (largest visible window it owns)
#   firstChange - the pixels first changed from the blank window: the app's UI arrived
#   lastChange  - the last change before the pixels held still
#
#   powershell -File rune-gui/scripts/measure-paint.ps1 -Exe src-tauri/target/release/droidgear.exe
#
# It raises the window (`SwitchToThisWindow`, which gets past the foreground lock a background
# caller runs into) and reads the desktop, so it needs an unlocked, visible desktop and ~25 ms
# between samples. Do not touch the mouse or keyboard while it runs.
#
# `-Method print` asks the window to draw itself with `PrintWindow` instead, which works while other
# windows cover it but returns the last composited frame: on this machine it ran ~450 ms behind what
# the screen showed for a wgpu window, so it is only a fallback.

param(
    [Parameter(Mandatory = $true)][string]$Exe,
    [int]$Runs = 5,
    # The sampled frame's side, in pixels; 64 is plenty to notice a page appearing.
    [int]$Size = 64,
    # Mean channel change (0-255) for a sample to count as changed.
    [int]$PixelDelta = 8,
    # Fraction of the frame that must change to count as a change (3%).
    [double]$ChangeThreshold = 0.03,
    # How long the pixels must hold still before the run ends.
    [int]$StableForMs = 500,
    # Runs are abandoned after this long.
    [int]$TimeoutMs = 15000,
    [ValidateSet('screen', 'print')][string]$Method = 'screen'
)

$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Drawing
. (Join-Path $PSScriptRoot 'measure-window.ps1')

$exePath = (Resolve-Path $Exe).Path
$name = [System.IO.Path]::GetFileName($exePath)
Write-Host "Sampling $name ($Method)" -ForegroundColor Cyan

# One frame, downsampled to $Size x $Size, read through LockBits: GetPixel per pixel costs ~400 ms a
# frame, which would quantise every measurement to that.
function Get-Frame($handle, $rect) {
    $w = $rect.Right - $rect.Left
    $h = $rect.Bottom - $rect.Top
    if ($w -le 0 -or $h -le 0) { return $null }
    $full = New-Object System.Drawing.Bitmap $w, $h
    $g = [System.Drawing.Graphics]::FromImage($full)
    if ($Method -eq 'print') {
        $dc = $g.GetHdc()
        $ok = [DgWin]::PrintWindow($handle, $dc)
        $g.ReleaseHdc($dc)
        $g.Dispose()
        if (-not $ok) { $full.Dispose(); return $null }
    }
    else {
        $g.CopyFromScreen($rect.Left, $rect.Top, 0, 0, (New-Object System.Drawing.Size $w, $h))
        $g.Dispose()
    }
    $small = New-Object System.Drawing.Bitmap $Size, $Size, ([System.Drawing.Imaging.PixelFormat]::Format24bppRgb)
    $gs = [System.Drawing.Graphics]::FromImage($small)
    $gs.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBilinear
    $gs.DrawImage($full, 0, 0, $Size, $Size)
    $gs.Dispose()
    $full.Dispose()
    $locked = $small.LockBits((New-Object System.Drawing.Rectangle 0, 0, $Size, $Size),
        [System.Drawing.Imaging.ImageLockMode]::ReadOnly, [System.Drawing.Imaging.PixelFormat]::Format24bppRgb)
    $bytes = New-Object byte[] ($locked.Stride * $Size)
    [System.Runtime.InteropServices.Marshal]::Copy($locked.Scan0, $bytes, 0, $bytes.Length)
    $small.UnlockBits($locked)
    $small.Dispose()
    return , $bytes
}

function Compare-Frames($a, $b) {
    if ($null -eq $a -or $null -eq $b -or $a.Length -ne $b.Length) { return 0 }
    $changed = 0
    for ($i = 0; $i -lt $a.Length; $i += 3) {
        if ([math]::Abs($a[$i] - $b[$i]) -gt $PixelDelta) { $changed++ }
    }
    return $changed / ($a.Length / 3)
}

$results = @()
for ($run = 0; $run -le $Runs; $run++) {
    $watch = [System.Diagnostics.Stopwatch]::StartNew()
    $proc = Start-Process -FilePath $exePath -PassThru
    $handle = Wait-RealWindow $proc
    $visibleMs = $script:WindowWaitMs
    if ($handle -eq [IntPtr]::Zero) {
        Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
        Write-Warning 'no window appeared'
        continue
    }
    [DgWin]::Raise($handle)
    Start-Sleep -Milliseconds 100

    $rect = $script:WindowRect
    $previous = Get-Frame $handle $rect
    $firstChange = $null
    $lastChange = $null
    $quietSince = $watch.Elapsed.TotalMilliseconds
    while ($watch.Elapsed.TotalMilliseconds -lt $TimeoutMs) {
        Start-Sleep -Milliseconds 25
        # Re-read the rect: the window settles its position and size as it appears.
        $refreshed = New-Object DgWin+RECT
        [void][DgWin]::Biggest([uint32]$proc.Id, [ref]$refreshed)
        if ($refreshed.Right -gt $refreshed.Left) { $rect = $refreshed }
        $frame = Get-Frame $handle $rect
        $now = $watch.Elapsed.TotalMilliseconds
        if ($null -ne $frame) {
            $diff = Compare-Frames $previous $frame
            if ($diff -gt $ChangeThreshold) {
                if ($null -eq $firstChange) { $firstChange = $now }
                $lastChange = $now
                $quietSince = $now
            }
            $previous = $frame
        }
        if ($now - $quietSince -ge $StableForMs) { break }
    }

    Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
    Start-Sleep -Milliseconds 800
    if ($run -eq 0) {
        Write-Host "  warm-up: visible $([math]::Round($visibleMs, 0)) ms" -ForegroundColor DarkGray
        continue
    }
    if ($null -eq $firstChange) { Write-Warning 'run did not see the window change' }
    $results += [pscustomobject]@{
        Run       = $run
        VisibleMs = [math]::Round($visibleMs, 0)
        FirstMs   = [math]::Round($firstChange, 0)
        LastMs    = [math]::Round($lastChange, 0)
    }
}

function Median($values) {
    $sorted = @($values | Where-Object { $null -ne $_ -and $_ -gt 0 } | Sort-Object)
    if ($sorted.Count -eq 0) { return 'n/a' }
    $mid = [math]::Floor($sorted.Count / 2)
    if ($sorted.Count % 2) { return $sorted[$mid] }
    return ($sorted[$mid - 1] + $sorted[$mid]) / 2
}

$results | Format-Table -AutoSize
Write-Host ("median visible {0} ms | first change {1} ms | last change {2} ms" -f `
        (Median $results.VisibleMs), (Median $results.FirstMs), (Median $results.LastMs)) -ForegroundColor Green
