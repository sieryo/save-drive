#requires -Version 5
<#
.SYNOPSIS
  Remove build outputs (bin/ and dist/).
#>
[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $PSScriptRoot
foreach ($dir in @('bin', 'dist')) {
    $path = Join-Path $root $dir
    if (Test-Path $path) {
        Remove-Item -Recurse -Force $path
        Write-Host "removed: $path"
    }
}
Write-Host 'clean complete' -ForegroundColor Green
