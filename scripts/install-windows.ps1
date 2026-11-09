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

function Install-RubberBand {
    param([Parameter(Mandatory = $true)][string] $BinDir)

    $localExe = Join-Path $BinDir 'rubberband.exe'
    if (Test-Path -LiteralPath $localExe -PathType Leaf) {
        Write-Host 'Rubber Band is already installed alongside cusic.'
        return
    }
    if (Get-Command rubberband -ErrorAction SilentlyContinue) {
        Write-Host 'Rubber Band is already available on PATH.'
        return
    }

    # Official upstream distribution, pinned to the SHA-256 published in
    # ScoopInstaller/Main's rubberband.json manifest.
    $version = '4.0.0'
    $url = 'https://breakfastquay.com/files/releases/rubberband-4.0.0-gpl-executable-windows.zip'
    $expectedSha256 = 'f2d47fc64dbb42f6cc62edf7933ac4fa89d8f0ef8b9cf97b6afc263a7fe05644'

    $temp = Join-Path ([IO.Path]::GetTempPath()) ("cusic-rubberband-" + [guid]::NewGuid().ToString('N'))
    $archive = Join-Path $temp 'rubberband.zip'
    $unpacked = Join-Path $temp 'unpacked'
    New-Item -ItemType Directory -Path $temp -Force | Out-Null

    try {
        Write-Host "Downloading Rubber Band $version from the official website..."
        [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
        Invoke-WebRequest -Uri $url -OutFile $archive -UseBasicParsing -ErrorAction Stop
        $actualHash = (Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash
        if ($actualHash -ine $expectedSha256) {
            throw "Rubber Band download SHA-256 mismatch (got $actualHash). Nothing was installed."
        }
        Expand-Archive -LiteralPath $archive -DestinationPath $unpacked -ErrorAction Stop
        $candidates = @(Get-ChildItem -LiteralPath $unpacked -File -Recurse -Filter 'rubberband.exe')
        if ($candidates.Count -ne 1) {
            throw "Expected one rubberband.exe in the official archive; found $($candidates.Count)"
        }
        $distributionDir = $candidates[0].DirectoryName
        $payload = @(Get-ChildItem -LiteralPath $distributionDir -Force)
        if ($payload.Count -eq 0) { throw 'Rubber Band archive contains no files' }

        # Copy EXEs, DLLs and license files together: the executable can
        # require DLLs beside it. All files come from the verified ZIP.
        foreach ($item in $payload) {
            if ($item.Name -ieq 'cusic.exe') {
                throw 'Refusing to overwrite cusic.exe with an external archive'
            }
            Copy-Item -LiteralPath $item.FullName -Destination (Join-Path $BinDir $item.Name) -Recurse -Force -ErrorAction Stop
        }
        if (-not (Test-Path -LiteralPath $localExe -PathType Leaf)) {
            throw 'Rubber Band executable missing after installation'
        }
        Write-Host "Rubber Band $version installed in $BinDir"
    } finally {
        Remove-Item -LiteralPath $temp -Recurse -Force -ErrorAction SilentlyContinue
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

    # Always provision the optional-but-required-for-tempo-render CLI once.
    Install-RubberBand -BinDir $binDir

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

    Write-Host 'Installed cusic and Rubber Band successfully.'
    Write-Host 'Restart your terminal (and VS Code if using its integrated terminal), then run: cusic help'
    exit 0
} catch {
    Write-Error "cusic installation failed: $($_.Exception.Message)"
    exit 1
}
