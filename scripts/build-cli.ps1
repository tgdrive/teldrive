param(
    [string]$Version = 'v2.0.0-drive-es.1',
    [string]$Output = '',
    [string]$Go = 'go',
    [string]$Bun = 'bun'
)
$ErrorActionPreference = 'Stop'
$sourceRoot = Split-Path $PSScriptRoot -Parent
if (!$Output) { $Output = Join-Path $sourceRoot 'dist-release' }
$Output = [IO.Path]::GetFullPath($Output)
Push-Location $sourceRoot
$oldGoOs = $env:GOOS; $oldGoArch = $env:GOARCH; $oldCgo = $env:CGO_ENABLED
try {
    & $Bun run --cwd ui build
    if ($LASTEXITCODE -ne 0) { throw 'La compilación de la UI falló.' }
    $revision = (& git rev-parse HEAD).Trim()
    $timestamp = [DateTime]::UtcNow.ToString('yyyy-MM-ddTHH:mm:ssZ')
    foreach ($platform in 'windows', 'linux', 'darwin') {
        foreach ($architecture in 'amd64', 'arm64') {
            $env:GOOS = $platform; $env:GOARCH = $architecture; $env:CGO_ENABLED = '0'
            $directory = Join-Path $Output "$platform-$architecture"
            New-Item -ItemType Directory -Path $directory -Force | Out-Null
            $binary = if ($platform -eq 'windows') { 'teldrive.exe' } else { 'teldrive' }
            & $Go build -trimpath -ldflags "-s -w -X main.version=$Version -X main.commit=$revision -X main.date=$timestamp" -o (Join-Path $directory $binary) ./cmd/teldrive
            if ($LASTEXITCODE -ne 0) { throw "La compilación de $platform/$architecture falló." }
        }
    }
} finally {
    $env:GOOS = $oldGoOs; $env:GOARCH = $oldGoArch; $env:CGO_ENABLED = $oldCgo
    Pop-Location
}
