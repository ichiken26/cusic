# Build cusic and register its bin directory in the current Windows user's
# persistent PATH. Run once from the repository root via .\install.cmd.
# Administrator privileges are NOT required.
$ErrorActionPreference = 'Stop'

function Normalize-PathEntry {
    param([string] $Entry)
    if ([string]::IsNullOrWhiteSpace($Entry)) { return '' }
    $expanded = [Environment]::ExpandEnvironmentVariables($Entry.Trim().Trim('"'))
    try {
        return [IO.Path]::GetFullPath($expanded).TrimEnd([char[]]@('\', '/'))
    } catch {
        return $expanded.TrimEnd([char[]]@('\', '/'))
    }
}

try {
    $repoRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
    $binDir = [IO.Path]::GetFullPath((Join-Path $repoRoot 'bin'))
    $exe = Join-Path $binDir 'cusic.exe'

    $null = Get-Command go -ErrorAction Stop
    if (-not (Test-Path -LiteralPath $binDir -PathType Container)) {
        New-Item -ItemType Directory -Path $binDir -Force | Out-Null
    }

    Push-Location $repoRoot
    try {
        & go build -mod=readonly -o $exe ./cmd/cusic
        if ($LASTEXITCODE -ne 0) { throw "go build failed (exit code $LASTEXITCODE)" }
    } finally {
        Pop-Location
    }
    if (-not (Test-Path -LiteralPath $exe -PathType Leaf)) {
        throw "The build did not produce $exe"
    }

    # Read the *user* PATH, not $env:Path: the latter also includes machine and
    # temporary session entries and should not be persisted wholesale.
    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    $entries = @()
    if (-not [string]::IsNullOrEmpty($userPath)) {
        $entries = @($userPath -split ';' | Where-Object { $_ -ne '' })
    }
    $alreadyRegistered = $false
    $normalizedBin = Normalize-PathEntry $binDir
    foreach ($entry in $entries) {
        if ((Normalize-PathEntry $entry) -ieq $normalizedBin) {
            $alreadyRegistered = $true
            break
        }
    }

    if (-not $alreadyRegistered) {
        $updatedUserPath = (@($entries) + $binDir) -join ';'
        [Environment]::SetEnvironmentVariable('Path', $updatedUserPath, 'User')
        Write-Host "Added cusic to your user PATH: $binDir"
    } else {
        Write-Host "cusic is already registered in your user PATH: $binDir"
    }

    # Useful when invoking this script directly or dot-sourcing it in a shell.
    # A child process cannot change its parent PowerShell/VS Code environment.
    $processEntries = @($env:Path -split ';')
    if (-not @($processEntries | Where-Object { (Normalize-PathEntry $_) -ieq $normalizedBin }).Count) {
        $env:Path = $env:Path.TrimEnd(';') + ';' + $binDir
    }

    Write-Host 'Installed cusic successfully.'
    Write-Host 'Restart your terminal (and VS Code if using its integrated terminal), then run: cusic help'
    exit 0
} catch {
    Write-Error "cusic installation failed: $($_.Exception.Message)"
    exit 1
}
