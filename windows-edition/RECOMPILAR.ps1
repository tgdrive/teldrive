param([string]$Go = 'go', [string]$Bun = 'bun')
$ErrorActionPreference = 'Stop'
$buildRoot = $PSScriptRoot
$buildOutput = Join-Path $buildRoot 'dist-custom/windows-amd64'
New-Item -ItemType Directory -Force $buildOutput | Out-Null
function Invoke-BuildStep([string]$Program, [string[]]$Arguments) {
  & $Program @Arguments
  if ($LASTEXITCODE -ne 0) { throw "Falló $Program ($LASTEXITCODE)" }
}
Push-Location (Join-Path $buildRoot 'teldrive-ui')
try {
  Invoke-BuildStep $Bun @('install','--frozen-lockfile')
  Invoke-BuildStep $Bun @('run','typecheck')
  Invoke-BuildStep $Bun @('test')
  Invoke-BuildStep $Bun @('run','build')
} finally { Pop-Location }
Push-Location (Join-Path $buildRoot 'teldrive')
try {
  $env:CGO_ENABLED = '0'
  Invoke-BuildStep $Go @('generate','./...')
  New-Item -ItemType Directory -Force 'ui/dist' | Out-Null
  Copy-Item -Path '../teldrive-ui/dist/*' -Destination 'ui/dist' -Recurse -Force
  Invoke-BuildStep $Go @('test','./cmd','./internal/...','./pkg/...','./tools/desktop')
  Invoke-BuildStep $Go @('vet','./cmd','./internal/...','./pkg/...','./tools/desktop')
  $serverPath = Join-Path $buildOutput 'teldrive.exe'
  Invoke-BuildStep $Go @('build','-trimpath','-ldflags','-s -w -X github.com/tgdrive/teldrive/internal/version.Version=1.8.3-drive-es -X github.com/tgdrive/teldrive/internal/version.CommitSHA=d400a2d-custom','-o',$serverPath,'.')
  $bundlePath = Join-Path (Get-Location).Path 'tools/desktop/bundle'
  $buildInput = [IO.File]::OpenRead($serverPath)
  $buildCompressed = [IO.File]::Create((Join-Path $bundlePath 'teldrive.exe.gz'))
  $buildGzip = [IO.Compression.GZipStream]::new($buildCompressed,[IO.Compression.CompressionLevel]::Optimal)
  try { $buildInput.CopyTo($buildGzip) } finally { $buildGzip.Dispose(); $buildCompressed.Dispose(); $buildInput.Dispose() }
  Invoke-BuildStep $Bun @('-e',"import schema from '../teldrive-ui/src/config/cli-schema.json'; import {cliLabels} from '../teldrive-ui/src/config/cli-labels'; await Bun.write('./tools/desktop/bundle/settings.json',JSON.stringify(schema.map((f,i)=>({...f,label:cliLabels[i]}))));")
  Invoke-BuildStep $Go @('build','-trimpath','-ldflags','-s -w -H windowsgui','-o',(Join-Path $buildOutput 'Teldrive Desktop.exe'),'./tools/desktop')
} finally { Pop-Location }
Write-Output "Compilación terminada: $buildOutput"
