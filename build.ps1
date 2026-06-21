# Windows Task Manager — production build script
# Produces a single optimized exe with no console window.
#
# Usage:
#   .\build.ps1                      # builds wtm.exe tagged as 0.3.0
#   .\build.ps1 -Version 0.3.0       # builds wtm.exe tagged as 0.3.0
#   .\build.ps1 -Out wtm-0.1.0.exe   # write to a specific file name

param(
    [string]$Version = "0.4.0",
    [string]$Out = "wtm.exe"
)

$ErrorActionPreference = "Stop"

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

Write-Host "==> tidying modules"
go mod tidy

Write-Host "==> verifying modules"
go mod verify

Write-Host "==> formatting"
go fmt ./...

Write-Host "==> testing"
go test ./... -count=1

Write-Host "==> vet"
go vet ./...

Write-Host "==> govulncheck"
go run golang.org/x/vuln/cmd/govulncheck@latest ./...

Write-Host "==> deadcode"
go run golang.org/x/tools/cmd/deadcode@latest ./...

Write-Host "==> unparam"
go run mvdan.cc/unparam@latest ./...

$gcc = Get-Command gcc -ErrorAction SilentlyContinue
if ($null -ne $gcc) {
    Write-Host "==> race"
    $env:CGO_ENABLED = "1"
    go test -race ./...
} else {
    Write-Host "==> race skipped (gcc not found; install MinGW/MSYS2 to enable go test -race)"
}

Write-Host "==> building $Out (version $Version)"
$env:GOOS = "windows"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "0"

$ldflags = "-s -w -H windowsgui -X main.version=$Version"
go build -trimpath -ldflags $ldflags -o $Out $Module

if (Test-Path $Out) {
    $size = (Get-Item $Out).Length / 1MB
    $sha = (Get-FileHash $Out -Algorithm SHA256).Hash.ToLower()
    Write-Host ("==> done: {0} ({1:N1} MB)" -f $Out, $size)
    Write-Host "==> sha256: $sha"
} else {
    Write-Error "build failed"
    exit 1
}
