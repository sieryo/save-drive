#requires -Version 5
<#
.SYNOPSIS
  Build an optimized, version-stamped release binary of save-drive.
.DESCRIPTION
  Produces dist/save-drive.exe with debug info stripped (-s -w), reproducible
  build paths (-trimpath), and version metadata stamped via -ldflags. The version
  defaults to `git describe`, or can be overridden with -Version.
.PARAMETER Version
  Version string to stamp. Defaults to `git describe --tags --always --dirty`.
.PARAMETER Output
  Output directory for the binary. Defaults to 'dist'.
.EXAMPLE
  ./scripts/release.ps1
.EXAMPLE
  ./scripts/release.ps1 -Version 1.2.0
#>
[CmdletBinding()]
param(
    [string]$Version,
    [string]$Output = 'dist'
)

$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

# Get-GitValue runs git safely: a non-zero exit or missing git yields the fallback
# rather than aborting the build (e.g. when there are no tags yet).
function Get-GitValue([string[]]$GitArgs, [string]$Fallback) {
    $old = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try {
        $out = & git @GitArgs 2>$null
        if ($LASTEXITCODE -eq 0 -and $out) {
            return ($out | Select-Object -First 1).ToString().Trim()
        }
    } catch { }
    finally { $ErrorActionPreference = $old }
    return $Fallback
}

if (-not $Version) {
    $Version = Get-GitValue @('describe', '--tags', '--always', '--dirty') 'dev'
}
$commit = Get-GitValue @('rev-parse', '--short', 'HEAD') 'none'
$date = [DateTime]::UtcNow.ToString('yyyy-MM-ddTHH:mm:ssZ')

$pkg = 'save-drive/internal/buildinfo'
$ldflags = "-s -w -X $pkg.Version=$Version -X $pkg.Commit=$commit -X $pkg.Date=$date"

$outDir = Join-Path $root $Output
$exe = Join-Path $outDir 'save-drive.exe'
New-Item -ItemType Directory -Force -Path $outDir | Out-Null

Write-Host '==> Building release binary' -ForegroundColor Cyan
Write-Host "    version=$Version commit=$commit date=$date"

& go build -trimpath -ldflags $ldflags -o $exe ./cmd/save-drive
if ($LASTEXITCODE -ne 0) { throw "go build failed (exit $LASTEXITCODE)" }

$item = Get-Item $exe
$sizeMB = [math]::Round($item.Length / 1MB, 2)
Write-Host ''
Write-Host '==> Release build complete' -ForegroundColor Green
Write-Host "    File:   $($item.FullName)"
Write-Host "    Size:   $sizeMB MB"
Write-Host "    Folder: $outDir"
