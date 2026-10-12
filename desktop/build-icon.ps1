param([string]$Source = (Join-Path $PSScriptRoot 'Assets\Teldrive.png'))
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Drawing
$destination = Join-Path $PSScriptRoot 'Assets'
New-Item -ItemType Directory -Path $destination -Force | Out-Null
$target = Join-Path $destination 'Teldrive.png'
if ([IO.Path]::GetFullPath($Source) -ne [IO.Path]::GetFullPath($target)) { Copy-Item -LiteralPath $Source -Destination $target }
$picture = [System.Drawing.Image]::FromFile($target)
$entries = @()
try {
    foreach ($size in @(16,24,32,48,64,128,256)) {
        $bitmap = [System.Drawing.Bitmap]::new($size, $size)
        $graphics = [System.Drawing.Graphics]::FromImage($bitmap)
        $buffer = [IO.MemoryStream]::new()
        try {
            $graphics.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
            $graphics.DrawImage($picture, 0, 0, $size, $size)
            $bitmap.Save($buffer, [System.Drawing.Imaging.ImageFormat]::Png)
            $entries += @{Size=$size; Data=$buffer.ToArray()}
        } finally { $buffer.Dispose(); $graphics.Dispose(); $bitmap.Dispose() }
    }
    $writer = [IO.BinaryWriter]::new([IO.File]::Create((Join-Path $destination 'Teldrive.ico')))
    try {
        $writer.Write([uint16]0); $writer.Write([uint16]1); $writer.Write([uint16]$entries.Count)
        $offset = 6 + 16 * $entries.Count
        foreach ($entry in $entries) {
            $writer.Write([byte]($entry.Size % 256)); $writer.Write([byte]($entry.Size % 256)); $writer.Write([byte]0); $writer.Write([byte]0)
            $writer.Write([uint16]1); $writer.Write([uint16]32); $writer.Write([uint32]$entry.Data.Length); $writer.Write([uint32]$offset)
            $offset += $entry.Data.Length
        }
        foreach ($entry in $entries) { $writer.Write([byte[]]$entry.Data) }
    } finally { $writer.Dispose() }
} finally { $picture.Dispose() }
Write-Output 'Teldrive.ico generado con siete tamaños y transparencia PNG.'
