# Compares how long an app takes to put its real UI on screen, by watching its pixels.
#
# `measure-startup.ps1` times the window becoming visible. That flatters a WebView app that shows a
# loader first and paints the UI later, so this script samples the window at 25 ms and reports:
#
#   visible  - the window became visible (for a WebView app this is often just the loader)
#   content  - the pixels first changed from what was there when the window appeared
#   settled  - the pixels stopped changing for 500 ms (the UI is done arriving)
#
#   powershell -File rune-gui/scripts/measure-paint.ps1 -Exe src-tauri/target/release/droidgear.exe
#
# Keep the desktop clear: it reads the screen, so an overlapping window counts as a change.

param(
    [Parameter(Mandatory = $true)][string]$Exe,
    [int]$Runs = 5,
    [int]$Size = 64,
    [double]$ChangeThreshold = 0.03,
    [double]$StableThreshold = 0.005,
    [int]$StableForMs = 500
)

$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Drawing
Add-Type @"
using System;
using System.Runtime.InteropServices;
public static class W32 {
    [StructLayout(LayoutKind.Sequential)] public struct RECT { public int Left, Top, Right, Bottom; }
    [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr hWnd);
    [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr hWnd, out RECT rect);
    [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr hWnd);
}
"@

$exePath = (Resolve-Path $Exe).Path
$name = [System.IO.Path]::GetFileName($exePath)
Write-Host "Sampling $name" -ForegroundColor Cyan

function Get-Frame([W32+RECT]$rect) {
    $w = $rect.Right - $rect.Left
    $h = $rect.Bottom - $rect.Top
    $bmp = New-Object System.Drawing.Bitmap $Size, $Size
    $g = [System.Drawing.Graphics]::FromImage($bmp)
    $g.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBilinear
    $g.CopyFromScreen($rect.Left, $rect.Top, 0, 0, (New-Object System.Drawing.Size $w, $h))
    $g.Dispose()
    $data = @()
    for ($y = 0; $y -lt $Size; $y++) {
        for ($x = 0; $x -lt $Size; $x++) {
            $c = $bmp.GetPixel($x, $y)
            $data += [int](($c.R + $c.G + $c.B) / 3)
        }
    }
    $bmp.Dispose()
    return $data
}

function Compare-Frames($a, $b) {
    $changed = 0
    for ($i = 0; $i -lt $a.Count; $i++) {
        if ([math]::Abs($a[$i] - $b[$i]) -gt 8) { $changed++ }
    }
    return $changed / $a.Count
}

$results = @()
for ($run = 0; $run -le $Runs; $run++) {
    $proc = Start-Process -FilePath $exePath -PassThru
    $watch = [System.Diagnostics.Stopwatch]::StartNew()
    $handle = [IntPtr]::Zero
    while ($watch.Elapsed.TotalSeconds -lt 20) {
        $proc.Refresh()
        if ($proc.HasExited) { break }
        if ($proc.MainWindowHandle -ne [IntPtr]::Zero -and [W32]::IsWindowVisible($proc.MainWindowHandle)) {
            $handle = $proc.MainWindowHandle
            break
        }
        Start-Sleep -Milliseconds 4
    }
    $visibleMs = $watch.Elapsed.TotalMilliseconds
    [void][W32]::SetForegroundWindow($handle)

    $rect = New-Object W32+RECT
    [void][W32]::GetWindowRect($handle, [ref]$rect)
    $previous = Get-Frame $rect
    $contentMs = $null
    $settledMs = $null
    $stableSince = $null
    while ($watch.Elapsed.TotalMilliseconds -lt 15000) {
        Start-Sleep -Milliseconds 25
        $frame = Get-Frame $rect
        $diff = Compare-Frames $previous $frame
        $now = $watch.Elapsed.TotalMilliseconds
        if ($null -eq $contentMs -and $diff -gt $ChangeThreshold) { $contentMs = $now }
        if ($diff -lt $StableThreshold) {
            if ($null -eq $stableSince) { $stableSince = $now }
            elseif ($now - $stableSince -ge $StableForMs) { $settledMs = $now; break }
        }
        else { $stableSince = $null }
        $previous = $frame
    }

    Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
    Start-Sleep -Milliseconds 800

    if ($run -eq 0) {
        Write-Host "  warm-up: visible $([math]::Round($visibleMs, 0)) ms" -ForegroundColor DarkGray
        continue
    }
    $results += [pscustomobject]@{
        Run       = $run
        VisibleMs = [math]::Round($visibleMs, 0)
        ContentMs = [math]::Round($contentMs, 0)
        SettledMs = [math]::Round($settledMs, 0)
    }
}

function Median($values) {
    $sorted = @($values | Where-Object { $null -ne $_ } | Sort-Object)
    if ($sorted.Count -eq 0) { return 'n/a' }
    $mid = [math]::Floor($sorted.Count / 2)
    if ($sorted.Count % 2) { return $sorted[$mid] }
    return ($sorted[$mid - 1] + $sorted[$mid]) / 2
}

$results | Format-Table -AutoSize
Write-Host ("median visible {0} ms | content {1} ms | settled {2} ms" -f `
        (Median $results.VisibleMs), (Median $results.ContentMs), (Median $results.SettledMs)) -ForegroundColor Green
