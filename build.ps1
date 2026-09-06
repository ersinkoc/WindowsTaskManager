# Windows Task Manager — production build script
# Produces a single optimized exe with no console window.
#
# Usage:
#   .\build.ps1                      # builds wtm.exe tagged as 0.4.0
#   .\build.ps1 -Version 0.4.0       # builds wtm.exe tagged as 0.4.0
#   .\build.ps1 -Out wtm-0.1.0.exe   # write to a specific file name

param(
    [string]$Version = "0.4.0",
    [string]$Out = "wtm.exe"
)

$ErrorActionPreference = "Stop"
# On PowerShell 7.4+ this makes native (go.exe) non-zero exits throw under
# $ErrorActionPreference = "Stop". The explicit check inside Invoke-Step
# covers older hosts, where a failed gate step would otherwise be ignored
# and the script would happily build the exe anyway.
$PSNativeCommandUseErrorActionPreference = $true

if ($Version -notmatch '^\d+\.\d+\.\d+(-[\w\.]+)?$') {
    Write-Error "Version must look like 0.1.0 or 0.1.0-rc.1"
    exit 1
}

$Root = Split-Path -Parent $MyInvocation.MyCommand.Definition
Set-Location $Root

if (-not [System.IO.Path]::IsPathRooted($Out)) {
    $Out = Join-Path $Root $Out
}
$Module = "./cmd/wtm"

function Invoke-Step([string]$Name, [scriptblock]$Body) {
    Write-Host "==> $Name"
    & $Body
    if ($LASTEXITCODE -ne 0) {
        Write-Error "$Name failed (exit $LASTEXITCODE)"
        exit $LASTEXITCODE
    }
}

Invoke-Step "tidying modules" { go mod tidy }
Invoke-Step "verifying modules" { go mod verify }
Invoke-Step "formatting" { go fmt ./... }
Invoke-Step "testing" { go test ./... -count=1 }
Invoke-Step "vet" { go vet ./... }
Invoke-Step "govulncheck" { go run golang.org/x/vuln/cmd/govulncheck@latest ./... }
Invoke-Step "deadcode" { go run golang.org/x/tools/cmd/deadcode@latest ./... }
Invoke-Step "unparam" { go run mvdan.cc/unparam@latest ./... }

$gcc = Get-Command gcc -ErrorAction SilentlyContinue
if ($null -ne $gcc) {
    Invoke-Step "race" {
        $env:CGO_ENABLED = "1"
        go test -race ./...
    }
} else {
    Write-Host "==> race skipped (gcc not found; install MinGW/MSYS2 to enable go test -race)"
}

Write-Host "==> building $Out (version $Version)"
$env:GOOS = "windows"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "0"

$ldflags = "-s -w -H windowsgui -X main.version=$Version"
go build -trimpath -ldflags $ldflags -o $Out $Module
if ($LASTEXITCODE -ne 0) {
    Write-Error "build failed"
    exit 1
}

if (Test-Path $Out) {
    $size = (Get-Item $Out).Length / 1MB
    $sha = (Get-FileHash $Out -Algorithm SHA256).Hash.ToLower()
    Write-Host ("==> done: {0} ({1:N1} MB)" -f $Out, $size)
    Write-Host "==> sha256: $sha"
} else {
    Write-Error "build failed"
    exit 1
}
