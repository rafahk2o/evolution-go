$ErrorActionPreference = "Stop"

$Root = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
$Dist = [IO.Path]::GetFullPath((Join-Path $Root "dist"))
$Artifacts = [IO.Path]::GetFullPath((Join-Path $Root "artifacts"))
$RootPrefix = $Root.TrimEnd([IO.Path]::DirectorySeparatorChar) + [IO.Path]::DirectorySeparatorChar

if (-not $Dist.StartsWith($RootPrefix, [StringComparison]::OrdinalIgnoreCase)) {
  throw "Unsafe dist path: $Dist"
}
if (-not $Artifacts.StartsWith($RootPrefix, [StringComparison]::OrdinalIgnoreCase)) {
  throw "Unsafe artifacts path: $Artifacts"
}

Push-Location $Root
try {
  & node --test "tests/*.test.js"
  if ($LASTEXITCODE -ne 0) { throw "Extension tests failed." }
  & node "scripts/validate.mjs"
  if ($LASTEXITCODE -ne 0) { throw "Extension validation failed." }

  if (Test-Path -LiteralPath $Dist) { Remove-Item -LiteralPath $Dist -Recurse -Force }
  if (Test-Path -LiteralPath $Artifacts) { Remove-Item -LiteralPath $Artifacts -Recurse -Force }
  New-Item -ItemType Directory -Path $Dist | Out-Null
  New-Item -ItemType Directory -Path (Join-Path $Dist "shared") | Out-Null
  New-Item -ItemType Directory -Path $Artifacts | Out-Null

  $RuntimeFiles = @(
    "manifest.json",
    "service-worker.js",
    "call-window.html",
    "call-window.css",
    "call-window.js",
    "call-controller.js",
    "recording.js",
    "audio-worklet.js"
  )
  foreach ($File in $RuntimeFiles) {
    Copy-Item -LiteralPath (Join-Path $Root $File) -Destination (Join-Path $Dist $File)
  }
  Copy-Item -LiteralPath (Join-Path $Root "shared/core.js") -Destination (Join-Path $Dist "shared/core.js")
  Copy-Item -LiteralPath (Join-Path $Root "shared/protocol.js") -Destination (Join-Path $Dist "shared/protocol.js")
  Copy-Item -LiteralPath (Join-Path $Root "icons") -Destination (Join-Path $Dist "icons") -Recurse

  $Manifest = Get-Content -Raw -LiteralPath (Join-Path $Root "manifest.json") | ConvertFrom-Json
  $Zip = Join-Path $Artifacts ("evolution-go-wacalls-browser-{0}.zip" -f $Manifest.version)
  Compress-Archive -Path (Join-Path $Dist "*") -DestinationPath $Zip -CompressionLevel Optimal

  Write-Host "Unpacked extension: $Dist"
  Write-Host "ZIP package: $Zip"
} finally {
  Pop-Location
}
