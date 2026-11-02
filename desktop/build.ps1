$ErrorActionPreference = 'Stop'
Push-Location $PSScriptRoot
try {
    $env:GOCACHE = Join-Path $env:TEMP 'Madar-Go-Build-Cache'
    go test .
    if ($LASTEXITCODE -ne 0) { throw 'Verification failed' }
    go build -tags 'desktop,production' -ldflags '-s -w -H windowsgui' -o '..\Nawa.exe' .
    if ($LASTEXITCODE -ne 0) { throw 'Build failed' }
} finally {
    Pop-Location
}

