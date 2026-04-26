param(
    [Parameter(Mandatory = $true)]
    [string]$IsccPath,
    [Parameter(Mandatory = $true)]
    [string]$AppVersion,
    [Parameter(Mandatory = $true)]
    [string]$RepoRoot,
    [Parameter(Mandatory = $true)]
    [string]$OutputDir,
    [Parameter(Mandatory = $true)]
    [string]$ScriptPath
)

function Normalize-WindowsPath {
    param([string]$Path)

    if ($Path -match '^[A-Za-z]:\\') {
        return $Path
    }

    if ($Path -match '^\\([A-Za-z])\\') {
        return ($Path.Substring(1,1) + ':' + $Path.Substring(2))
    }

    if ($Path -match '^/([A-Za-z])/(.*)$') {
        return ($matches[1] + ':\\' + ($matches[2] -replace '/', '\\'))
    }

    return $Path
}

$IsccPath = Normalize-WindowsPath $IsccPath
$RepoRoot = Normalize-WindowsPath $RepoRoot
$OutputDir = Normalize-WindowsPath $OutputDir
$ScriptPath = Normalize-WindowsPath $ScriptPath

& $IsccPath "/DAppVersion=$AppVersion" "/DRepoRoot=$RepoRoot" "/DOutputDir=$OutputDir" $ScriptPath
if (-not $?) {
    exit 1
}
