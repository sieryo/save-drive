#requires -Version 5
<#
.SYNOPSIS
  Build a development binary of save-drive and optionally run it.
.DESCRIPTION
  Produces bin/save-drive.exe from ./cmd/save-drive (fast, unstripped build).
  Any arguments passed to the script are forwarded to the freshly built binary.
.EXAMPLE
  ./scripts/dev.ps1
.EXAMPLE
  ./scripts/dev.ps1 list --from Test
#>
[CmdletBinding()]
param(
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$RunArgs
)

$ErrorActionPreference = 'Stop'

# Resolve repo root (parent of this scripts/ dir) so the script works from anywhere.
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

$outDir = Join-Path $root 'bin'
$exe = Join-Path $outDir 'save-drive.exe'
New-Item -ItemType Directory -Force -Path $outDir | Out-Null

Write-Host '==> Building dev binary...' -ForegroundColor Cyan
& go build -o $exe ./cmd/save-drive
if ($LASTEXITCODE -ne 0) { throw "go build failed (exit $LASTEXITCODE)" }

$full = (Resolve-Path $exe).Path
Write-Host '==> Build complete' -ForegroundColor Green
Write-Host "    File:   $full"
Write-Host "    Folder: $outDir"

if ($RunArgs -and $RunArgs.Count -gt 0) {
    Write-Host "==> Running: save-drive $($RunArgs -join ' ')" -ForegroundColor Cyan
    & $exe @RunArgs
    exit $LASTEXITCODE
}
