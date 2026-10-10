param(
  [string]$Version = "",
  [string]$InstallDir = "$env:LOCALAPPDATA\Teldrive\bin"
)
$ErrorActionPreference = "Stop"
if ([Environment]::OSVersion.Platform -ne "Win32NT") { throw "This installer supports Windows only." }
$arch = switch ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()) {
  "X64" { "amd64" }
  "Arm64" { "arm64" }
  default { throw "Unsupported architecture." }
}
if (!$Version) {
  $release = Invoke-RestMethod "https://api.github.com/repos/tgdrive/teldrive/releases/latest"
  if ($release.draft -or $release.prerelease) { throw "No stable published release found." }
  $Version = $release.tag_name
}
if ($Version -notmatch '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$' -or $Version -match '^[01]\.') { throw "A stable Teldrive 2.x or newer release is required." }
$work = Join-Path ([IO.Path]::GetTempPath()) ([Guid]::NewGuid().ToString())
New-Item -ItemType Directory $work | Out-Null
try {
  $asset = "teldrive-$Version-windows-$arch.zip"
  $base = "https://github.com/tgdrive/teldrive/releases/download/$Version"
  $archive = Join-Path $work $asset
  Invoke-WebRequest "$base/$asset" -OutFile $archive
  Invoke-WebRequest "$base/teldrive_checksums.txt" -OutFile "$work\checksums"
  $lines = @(Get-Content "$work\checksums" | Where-Object { ($_ -split '\s+')[1] -eq $asset })
  if ($lines.Count -ne 1) { throw "Missing or ambiguous checksum." }
  $expected = ($lines[0] -split '\s+')[0]
  if ((Get-FileHash $archive -Algorithm SHA256).Hash -ne $expected) { throw "Checksum mismatch." }
  Expand-Archive $archive -DestinationPath "$work\extracted"
  New-Item -ItemType Directory -Force $InstallDir | Out-Null
  Copy-Item "$work\extracted\teldrive.exe" "$InstallDir\teldrive.exe" -Force
  $completion = & "$InstallDir\teldrive.exe" completion powershell
  if ($LASTEXITCODE -ne 0) { throw "Completion generation failed." }
  $completion | Set-Content "$InstallDir\teldrive-completion.ps1"
  Write-Output "Installed Teldrive $Version to $InstallDir. Add it to PATH."
  Write-Output "To enable completion, source $InstallDir\teldrive-completion.ps1 in your PowerShell profile."
} finally {
  Remove-Item $work -Recurse -Force
}
