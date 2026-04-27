[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$RepoRoot,

    [Parameter(Mandatory = $true)]
    [string]$AppVersion,

    [Parameter(Mandatory = $true)]
    [string]$IdentityName,

    [Parameter(Mandatory = $true)]
    [string]$Publisher,

    [Parameter(Mandatory = $true)]
    [string]$PublisherDisplayName,

    [Parameter(Mandatory = $true)]
    [string]$DisplayName,

    [string]$StagedBuildDir,

    [string]$OutputDir,

    [string]$ManifestTemplate,

    [string]$AssetsDir,

    [switch]$TestSign,

    [string]$PfxPath,

    [string]$PfxPassword
)

$ErrorActionPreference = 'Stop'

function Resolve-RepoPath {
    param([string]$Path, [string]$Description)

    if ([string]::IsNullOrWhiteSpace($Path)) {
        throw "fatal: empty path for $Description"
    }

    if ($Path -match '^/([A-Za-z])/(.*)$') {
        $Path = ($matches[1] + ':\' + ($matches[2] -replace '/', '\'))
    } elseif ($Path -match '^\\([A-Za-z])\\') {
        $Path = ($Path.Substring(1,1) + ':' + $Path.Substring(2))
    }

    return $Path
}

function Convert-ToStoreVersion {
    param([string]$Raw)

    # Strip dev suffix like -dev-abc1234[.dirty]; Store rejects non-numeric versions.
    $clean = ($Raw -split '-')[0]
    $parts = $clean -split '\.'
    if ($parts.Count -lt 3 -or $parts.Count -gt 4) {
        throw "fatal: cannot normalize app version '$Raw' to Store 4-part form"
    }
    foreach ($p in $parts) {
        if (-not ($p -match '^\d+$')) {
            throw "fatal: app version '$Raw' contains non-numeric segment '$p'"
        }
    }
    if ($parts.Count -eq 3) {
        return ($clean + '.0')
    }
    return $clean
}

function Find-MakeAppx {
    $cmd = Get-Command 'MakeAppx.exe' -ErrorAction SilentlyContinue
    if ($cmd) { return $cmd.Source }

    $sdkRoots = @(
        'C:\Program Files (x86)\Windows Kits\10\bin',
        'C:\Program Files\Windows Kits\10\bin'
    )
    foreach ($root in $sdkRoots) {
        if (-not (Test-Path -LiteralPath $root)) { continue }
        $candidates = Get-ChildItem -LiteralPath $root -Directory -ErrorAction SilentlyContinue |
            Where-Object { $_.Name -match '^10\.' } |
            Sort-Object Name -Descending
        foreach ($dir in $candidates) {
            $p = Join-Path $dir.FullName 'x64\MakeAppx.exe'
            if (Test-Path -LiteralPath $p) { return $p }
        }
    }
    throw "fatal: MakeAppx.exe not found. Install the Windows 10/11 SDK or add MakeAppx.exe to PATH."
}

function Find-SignTool {
    $cmd = Get-Command 'signtool.exe' -ErrorAction SilentlyContinue
    if ($cmd) { return $cmd.Source }

    $sdkRoots = @(
        'C:\Program Files (x86)\Windows Kits\10\bin',
        'C:\Program Files\Windows Kits\10\bin'
    )
    foreach ($root in $sdkRoots) {
        if (-not (Test-Path -LiteralPath $root)) { continue }
        $candidates = Get-ChildItem -LiteralPath $root -Directory -ErrorAction SilentlyContinue |
            Where-Object { $_.Name -match '^10\.' } |
            Sort-Object Name -Descending
        foreach ($dir in $candidates) {
            $p = Join-Path $dir.FullName 'x64\signtool.exe'
            if (Test-Path -LiteralPath $p) { return $p }
        }
    }
    throw "fatal: signtool.exe not found. Install the Windows 10/11 SDK or add signtool.exe to PATH."
}

function Generate-PlaceholderPng {
    param(
        [string]$IcoPath,
        [string]$OutPath,
        [int]$Width,
        [int]$Height
    )

    Add-Type -AssemblyName System.Drawing
    $sourceIcon = New-Object System.Drawing.Icon($IcoPath, $Width, $Height)
    try {
        $bmp = New-Object System.Drawing.Bitmap($Width, $Height)
        try {
            $g = [System.Drawing.Graphics]::FromImage($bmp)
            try {
                $g.Clear([System.Drawing.Color]::Transparent)
                $g.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
                $iconBmp = $sourceIcon.ToBitmap()
                try {
                    $iconSize = [Math]::Min($Width, $Height)
                    $offsetX = [int](($Width - $iconSize) / 2)
                    $offsetY = [int](($Height - $iconSize) / 2)
                    $g.DrawImage($iconBmp, $offsetX, $offsetY, $iconSize, $iconSize)
                } finally {
                    $iconBmp.Dispose()
                }
            } finally {
                $g.Dispose()
            }
            $bmp.Save($OutPath, [System.Drawing.Imaging.ImageFormat]::Png)
        } finally {
            $bmp.Dispose()
        }
    } finally {
        $sourceIcon.Dispose()
    }
}

# --- Resolve & validate inputs ----------------------------------------------

$RepoRoot = Resolve-RepoPath -Path $RepoRoot -Description 'RepoRoot'
if (-not (Test-Path -LiteralPath $RepoRoot)) {
    throw "fatal: RepoRoot does not exist: $RepoRoot"
}

if (-not $StagedBuildDir) {
    $StagedBuildDir = Join-Path $RepoRoot 'build\windows-amd64'
} else {
    $StagedBuildDir = Resolve-RepoPath -Path $StagedBuildDir -Description 'StagedBuildDir'
}
if (-not (Test-Path -LiteralPath $StagedBuildDir)) {
    throw "fatal: staged build dir not found: $StagedBuildDir. Run 'make build-windows-runtime-amd64' first."
}

if (-not $OutputDir) {
    $OutputDir = $StagedBuildDir
} else {
    $OutputDir = Resolve-RepoPath -Path $OutputDir -Description 'OutputDir'
}
New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null

if (-not $ManifestTemplate) {
    $ManifestTemplate = Join-Path $RepoRoot 'packaging\windows\msix\AppxManifest.xml.tmpl'
} else {
    $ManifestTemplate = Resolve-RepoPath -Path $ManifestTemplate -Description 'ManifestTemplate'
}
if (-not (Test-Path -LiteralPath $ManifestTemplate)) {
    throw "fatal: manifest template not found: $ManifestTemplate"
}

if (-not $AssetsDir) {
    $AssetsDir = Join-Path $RepoRoot 'packaging\windows\msix\Assets'
} else {
    $AssetsDir = Resolve-RepoPath -Path $AssetsDir -Description 'AssetsDir'
}
if (-not (Test-Path -LiteralPath $AssetsDir)) {
    throw "fatal: assets dir not found: $AssetsDir"
}

$icoPath = Join-Path $RepoRoot 'assets\windows\joicetyper.ico'
if (-not (Test-Path -LiteralPath $icoPath)) {
    throw "fatal: source icon missing: $icoPath"
}

$storeVersion = Convert-ToStoreVersion -Raw $AppVersion
Write-Host "MSIX version (store form): $storeVersion (from '$AppVersion')"

# --- Stage payload ----------------------------------------------------------

$stagingRoot = Join-Path $OutputDir 'msix-staging'
if (Test-Path -LiteralPath $stagingRoot) {
    Remove-Item -LiteralPath $stagingRoot -Recurse -Force
}
New-Item -ItemType Directory -Path $stagingRoot | Out-Null
$stagingAssets = Join-Path $stagingRoot 'Assets'
New-Item -ItemType Directory -Path $stagingAssets | Out-Null

$payloadFiles = @(
    'joicetyper.exe',
    'whisper.dll',
    'libwhisper.dll',
    'ggml.dll',
    'ggml-base.dll',
    'ggml-cpu.dll',
    'ggml-vulkan.dll',
    'libgcc_s_seh-1.dll',
    'libstdc++-6.dll',
    'libgomp-1.dll',
    'libdl.dll'
)
$optionalPayload = @('libwinpthread-1.dll')

foreach ($f in $payloadFiles) {
    $src = Join-Path $StagedBuildDir $f
    if (-not (Test-Path -LiteralPath $src)) {
        throw "fatal: missing staged payload file: $src"
    }
    Copy-Item -LiteralPath $src -Destination (Join-Path $stagingRoot $f) -Force
}
foreach ($f in $optionalPayload) {
    $src = Join-Path $StagedBuildDir $f
    if (Test-Path -LiteralPath $src) {
        Copy-Item -LiteralPath $src -Destination (Join-Path $stagingRoot $f) -Force
    }
}

# --- Stage assets (real or placeholder) -------------------------------------

$assetSpecs = @(
    @{ Name = 'StoreLogo.png';        Width = 50;  Height = 50  },
    @{ Name = 'Square44x44Logo.png';  Width = 44;  Height = 44  },
    @{ Name = 'Square150x150Logo.png';Width = 150; Height = 150 },
    @{ Name = 'Wide310x150Logo.png';  Width = 310; Height = 150 },
    @{ Name = 'SplashScreen.png';     Width = 620; Height = 300 }
)

foreach ($spec in $assetSpecs) {
    $real = Join-Path $AssetsDir $spec.Name
    $dest = Join-Path $stagingAssets $spec.Name
    if (Test-Path -LiteralPath $real) {
        Copy-Item -LiteralPath $real -Destination $dest -Force
    } else {
        Write-Host "warn: missing $($spec.Name) — generating placeholder from joicetyper.ico"
        Generate-PlaceholderPng -IcoPath $icoPath -OutPath $dest -Width $spec.Width -Height $spec.Height
    }
}

# Copy any scale-* / targetsize-* variants that the user has supplied.
Get-ChildItem -LiteralPath $AssetsDir -File -ErrorAction SilentlyContinue |
    Where-Object { $_.Name -match '\.(scale|targetsize)-' } |
    ForEach-Object {
        Copy-Item -LiteralPath $_.FullName -Destination (Join-Path $stagingAssets $_.Name) -Force
    }

# --- Render manifest --------------------------------------------------------

$manifestText = Get-Content -LiteralPath $ManifestTemplate -Raw
$manifestText = $manifestText.Replace('$IDENTITY_NAME$', $IdentityName)
$manifestText = $manifestText.Replace('$PUBLISHER$', $Publisher)
$manifestText = $manifestText.Replace('$PUBLISHER_DISPLAY_NAME$', $PublisherDisplayName)
$manifestText = $manifestText.Replace('$DISPLAY_NAME$', $DisplayName)
$manifestText = $manifestText.Replace('$VERSION$', $storeVersion)

$stagingManifest = Join-Path $stagingRoot 'AppxManifest.xml'
Set-Content -LiteralPath $stagingManifest -Value $manifestText -Encoding UTF8 -NoNewline

# --- Pack -------------------------------------------------------------------

$makeAppx = Find-MakeAppx
$msixName = "JoiceTyper-$storeVersion.msix"
$msixPath = Join-Path $OutputDir $msixName

Write-Host "Packing $msixPath"
& $makeAppx pack /d $stagingRoot /p $msixPath /o
if ($LASTEXITCODE -ne 0) {
    throw "fatal: MakeAppx.exe pack failed with exit code $LASTEXITCODE"
}

# --- Optional test-sign for sideload testing --------------------------------

if ($TestSign) {
    if (-not $PfxPath) {
        throw "fatal: -TestSign requires -PfxPath"
    }
    $PfxPath = Resolve-RepoPath -Path $PfxPath -Description 'PfxPath'
    if (-not (Test-Path -LiteralPath $PfxPath)) {
        throw "fatal: pfx not found: $PfxPath"
    }
    $signTool = Find-SignTool
    Write-Host "Test-signing $msixPath with $PfxPath"
    $signArgs = @('sign', '/fd', 'SHA256', '/a', '/f', $PfxPath)
    if ($PfxPassword) { $signArgs += @('/p', $PfxPassword) }
    $signArgs += $msixPath
    & $signTool @signArgs
    if ($LASTEXITCODE -ne 0) {
        throw "fatal: signtool.exe sign failed with exit code $LASTEXITCODE"
    }
} else {
    Write-Host "Skipping signing — Store submissions must be uploaded unsigned (Microsoft re-signs)."
}

Write-Host "MSIX ready: $msixPath"
