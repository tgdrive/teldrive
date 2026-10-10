param(
    [ValidateSet('podman', 'docker')][string]$Engine = 'podman',
    [string]$Image = 'ghcr.io/tgdrive/postgres:18',
    [string]$Executable = 'go',
    [string[]]$Arguments = @('test', '-tags=integration', './...')
)
$ErrorActionPreference = 'Stop'
Get-Command $Engine -ErrorAction Stop | Out-Null
$containerName = 'teldrive-test-' + [Guid]::NewGuid().ToString('N')
$previousDatabase = $env:TEST_DATABASE_URL
$started = $false
Push-Location (Split-Path $PSScriptRoot -Parent)
try {
    & $Engine run -d --name $containerName -e POSTGRES_USER=teldrive -e POSTGRES_PASSWORD=teldrive -e POSTGRES_DB=teldrive_test -p '127.0.0.1::5432' $Image | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'No se pudo iniciar PostgreSQL para las pruebas.' }
    $started = $true
    $mapping = (& $Engine port $containerName 5432/tcp) -join "`n"
    if ($mapping -notmatch '127\.0\.0\.1:(\d+)') { throw 'No se pudo obtener el puerto local de PostgreSQL.' }
    $databasePort = $Matches[1]
    $ready = $false
    $deadline = [DateTime]::UtcNow.AddSeconds(60)
    while ([DateTime]::UtcNow -lt $deadline) {
        $probe = & $Engine exec -e PGPASSWORD=teldrive $containerName psql -h 127.0.0.1 -U teldrive -d teldrive_test -Atqc 'SELECT 1' 2>$null
        if ($LASTEXITCODE -eq 0 -and ($probe -join '').Trim() -eq '1') { $ready = $true; break }
        Start-Sleep -Milliseconds 250
    }
    if (!$ready) { & $Engine logs $containerName; throw 'PostgreSQL no respondió a la consulta TCP.' }
    $env:TEST_DATABASE_URL = "postgres://teldrive:teldrive@127.0.0.1:$databasePort/teldrive_test?sslmode=disable"
    & $Executable @Arguments
    if ($LASTEXITCODE -ne 0) { throw "Las pruebas fallaron con código $LASTEXITCODE." }
} finally {
    $env:TEST_DATABASE_URL = $previousDatabase
    if ($started) { & $Engine rm -f $containerName | Out-Null }
    Pop-Location
}
