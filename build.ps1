# Build the Linux API binary and the static site, the same idea as
# C:\QSOURCE\_QtaggOnshore\onshore\build.ps1 -Linux.
# Run from the repo root:  .\build.ps1

$ErrorActionPreference = "Stop"
$root = $PSScriptRoot
$originalGoos = $env:GOOS

$env:GOOS = "linux"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "0"

try {
    Write-Output "Building frontend"
    Push-Location (Join-Path $root "frontend")
    npm install
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    if ($env:VITE_API_URL) { Remove-Item Env:VITE_API_URL }
    npm run build
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    Pop-Location

    Write-Output "Building linux/amd64 sol-backend"
    Push-Location (Join-Path $root "backend")
    go build -o (Join-Path $root "sol-backend") .
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    Pop-Location

    Write-Output "Done: sol-backend and frontend\dist"
}
finally {
    if ([string]::IsNullOrEmpty($originalGoos)) {
        Remove-Item Env:GOOS -ErrorAction SilentlyContinue
    } else {
        $env:GOOS = $originalGoos
    }
}
