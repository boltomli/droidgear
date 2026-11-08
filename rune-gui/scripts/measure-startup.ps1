# Measures a desktop app's launch: time to a visible window, memory once it settles, and the CPU
# it burns while idle. Run it twice for the two builds you want to compare, on the same machine,
# with the same workload on screen:
#
#   powershell -File rune-gui/scripts/measure-startup.ps1 -Exe src-tauri/target/release/droidgear.exe
#   powershell -File rune-gui/scripts/measure-startup.ps1 -Exe rune-gui/target/release/droidgear-gui.exe
#
# RSS is summed over the process and its descendants, so a WebView's helper processes count against
# the app that spawned them. CPU is a percentage of one core, sampled over `-IdleSeconds` once the
# window has settled.

param(
    [Parameter(Mandatory = $true)][string]$Exe,
    # Runs to time; the first one only warms the file cache.
    [int]$Runs = 5,
    # How long the app runs before the idle sample starts.
    [double]$SettleSeconds = 6,
    # How long the idle CPU sample lasts.
    [double]$IdleSeconds = 4
)

$ErrorActionPreference = 'Stop'
$exePath = (Resolve-Path $Exe).Path
$name = [System.IO.Path]::GetFileName($exePath)
Write-Host "Measuring $name ($([math]::Round((Get-Item $exePath).Length / 1MB, 1)) MB)" -ForegroundColor Cyan

# DroidGear (Tauri) creates its window hidden and shows it from React, so the handle exists before
# anything is on screen. Waiting for a visible window compares like with like.
Add-Type @"
using System;
using System.Runtime.InteropServices;
public static class Win32Visible {
    [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr hWnd);
}
"@

function Test-VisibleWindow($proc) {
    $proc.Refresh()
    if ($proc.MainWindowHandle -eq [IntPtr]::Zero) { return $false }
    return [Win32Visible]::IsWindowVisible($proc.MainWindowHandle)
}

function Get-Tree([int]$rootId) {
    $all = Get-CimInstance Win32_Process -Filter "ParentProcessId=$rootId" -ErrorAction SilentlyContinue
    $children = @()
    foreach ($child in $all) { $children += $child; $children += Get-Tree $child.ProcessId }
    return $children
}

function Get-TreeRss([int]$rootId) {
    $total = 0
    $process = Get-Process -Id $rootId -ErrorAction SilentlyContinue
    if ($process) { $total += $process.WorkingSet64 }
    foreach ($child in (Get-Tree $rootId)) {
        $p = Get-Process -Id $child.ProcessId -ErrorAction SilentlyContinue
        if ($p) { $total += $p.WorkingSet64 }
    }
    return $total
}

function Get-TreeCpu([int]$rootId) {
    $total = 0
    $process = Get-Process -Id $rootId -ErrorAction SilentlyContinue
    if ($process) { $total += $process.TotalProcessorTime.TotalMilliseconds }
    foreach ($child in (Get-Tree $rootId)) {
        $p = Get-Process -Id $child.ProcessId -ErrorAction SilentlyContinue
        if ($p) { $total += $p.TotalProcessorTime.TotalMilliseconds }
    }
    return $total
}

$results = @()
for ($run = 0; $run -le $Runs; $run++) {
    $proc = Start-Process -FilePath $exePath -PassThru
    $watch = [System.Diagnostics.Stopwatch]::StartNew()
    $windowMs = $null
    while ($watch.Elapsed.TotalSeconds -lt 20) {
        if ($proc.HasExited) { break }
        if (Test-VisibleWindow $proc) {
            $windowMs = $watch.Elapsed.TotalMilliseconds
            break
        }
        Start-Sleep -Milliseconds 4
    }

    Start-Sleep -Seconds $SettleSeconds
    $proc.Refresh()
    $rss = Get-TreeRss $proc.Id
    $cpu1 = Get-TreeCpu $proc.Id
    $t1 = [System.Diagnostics.Stopwatch]::StartNew()
    Start-Sleep -Seconds $IdleSeconds
    $proc.Refresh()
    $cpu2 = Get-TreeCpu $proc.Id
    $idleCpu = ($cpu2 - $cpu1) / $t1.Elapsed.TotalMilliseconds * 100
    $threads = $proc.Threads.Count
    $handles = $proc.HandleCount
    $procs = @(Get-Tree $proc.Id).Count + 1

    Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
    Start-Sleep -Milliseconds 700

    if ($run -eq 0) { Write-Host "  warm-up: window $([math]::Round($windowMs, 0)) ms" -ForegroundColor DarkGray; continue }

    $results += [pscustomobject]@{
        Run          = $run
        WindowMs     = [math]::Round($windowMs, 0)
        RssMb        = [math]::Round($rss / 1MB, 1)
        IdleCpuPct   = [math]::Round($idleCpu, 2)
        Threads      = $threads
        Handles      = $handles
        Processes    = $procs
    }
}

function Median($values) {
    $sorted = $values | Sort-Object
    $mid = [math]::Floor($sorted.Count / 2)
    if ($sorted.Count % 2) { return $sorted[$mid] }
    return ($sorted[$mid - 1] + $sorted[$mid]) / 2
}

$results | Format-Table -AutoSize
Write-Host ("median window {0} ms | rss {1} MB | idle cpu {2}% | processes {3}" -f `
        (Median $results.WindowMs), (Median $results.RssMb), (Median $results.IdleCpuPct), (Median $results.Processes)) `
    -ForegroundColor Green