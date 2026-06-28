$ErrorActionPreference = "Stop"

Add-Type -AssemblyName System.Drawing

$Root = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
$Icons = Join-Path $Root "icons"
New-Item -ItemType Directory -Path $Icons -Force | Out-Null

function New-RoundedRectanglePath {
  param(
    [float]$X,
    [float]$Y,
    [float]$Width,
    [float]$Height,
    [float]$Radius
  )

  $Path = [Drawing.Drawing2D.GraphicsPath]::new()
  $Diameter = $Radius * 2
  $Path.AddArc($X, $Y, $Diameter, $Diameter, 180, 90)
  $Path.AddArc($X + $Width - $Diameter, $Y, $Diameter, $Diameter, 270, 90)
  $Path.AddArc($X + $Width - $Diameter, $Y + $Height - $Diameter, $Diameter, $Diameter, 0, 90)
  $Path.AddArc($X, $Y + $Height - $Diameter, $Diameter, $Diameter, 90, 90)
  $Path.CloseFigure()
  return $Path
}

$Master = [Drawing.Bitmap]::new(512, 512, [Drawing.Imaging.PixelFormat]::Format32bppArgb)
$Graphics = [Drawing.Graphics]::FromImage($Master)
$Graphics.SmoothingMode = [Drawing.Drawing2D.SmoothingMode]::AntiAlias
$Graphics.PixelOffsetMode = [Drawing.Drawing2D.PixelOffsetMode]::HighQuality
$Graphics.Clear([Drawing.Color]::Transparent)

$Green = [Drawing.SolidBrush]::new([Drawing.ColorTranslator]::FromHtml("#00e995"))
$Dark = [Drawing.ColorTranslator]::FromHtml("#052318")
$Tile = New-RoundedRectanglePath -X 16 -Y 16 -Width 480 -Height 480 -Radius 112
$Graphics.FillPath($Green, $Tile)

$DarkBrush = [Drawing.SolidBrush]::new($Dark)
$Handset = New-RoundedRectanglePath -X 148 -Y 165 -Width 216 -Height 78 -Radius 28
$Base = New-RoundedRectanglePath -X 164 -Y 283 -Width 184 -Height 92 -Radius 24
$Graphics.FillPath($DarkBrush, $Handset)
$Graphics.FillEllipse($Green, 205, 204, 102, 80)
$Graphics.FillPath($DarkBrush, $Base)
$Graphics.FillEllipse($DarkBrush, 207, 214, 98, 98)
$Graphics.FillEllipse($Green, 235, 242, 42, 42)

$Handset.Dispose()
$Base.Dispose()
$DarkBrush.Dispose()
$Tile.Dispose()
$Green.Dispose()
$Graphics.Dispose()

foreach ($Size in @(16, 32, 48, 128)) {
  $Bitmap = [Drawing.Bitmap]::new($Size, $Size, [Drawing.Imaging.PixelFormat]::Format32bppArgb)
  $Output = [Drawing.Graphics]::FromImage($Bitmap)
  $Output.CompositingMode = [Drawing.Drawing2D.CompositingMode]::SourceCopy
  $Output.CompositingQuality = [Drawing.Drawing2D.CompositingQuality]::HighQuality
  $Output.InterpolationMode = [Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
  $Output.SmoothingMode = [Drawing.Drawing2D.SmoothingMode]::HighQuality
  $Output.PixelOffsetMode = [Drawing.Drawing2D.PixelOffsetMode]::HighQuality
  $Output.DrawImage($Master, 0, 0, $Size, $Size)
  $Output.Dispose()
  $Target = Join-Path $Icons ("icon-{0}.png" -f $Size)
  $Bitmap.Save($Target, [Drawing.Imaging.ImageFormat]::Png)
  $Bitmap.Dispose()
}

$Master.Dispose()
Write-Host "Extension icons generated in $Icons"
