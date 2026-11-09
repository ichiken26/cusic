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
        # Windows PowerShell may run with reduced cmdlet availability.
        # Compute SHA-256 via .NET rather than relying on Get-FileHash.
        $sha = [Security.Cryptography.SHA256]::Create()
        $stream = [IO.File]::OpenRead($archive)
        try {
            $hashBytes = $sha.ComputeHash($stream)
            $actualHash = ([BitConverter]::ToString($hashBytes)).Replace('-', '')
        } finally {
            $stream.Dispose()
            $sha.Dispose()
        }
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


# Both tools are private to this checkout. No system-wide Go/Python/uv
# installation, admin permissions, activation, or Docker Desktop required.
function Get-VerifiedArchive {
    param(
        [Parameter(Mandatory = $true)][string] $Url,
        [Parameter(Mandatory = $true)][string] $Sha256,
        [Parameter(Mandatory = $true)][string] $Destination
    )
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
    # curl.exe is bundled with modern Windows and streams large toolchain
    # archives substantially more efficiently than Windows PowerShell 5 IWR.
    $curl = Get-Command 'curl.exe' -CommandType Application -ErrorAction SilentlyContinue
    if ($curl) {
        & $curl.Source --fail --location --retry 3 --connect-timeout 30 --max-time 240 --silent --show-error --output $Destination $Url
        if ($LASTEXITCODE -ne 0) { throw "Download failed for $Url (curl exit code $LASTEXITCODE)" }
    } else {
        Invoke-WebRequest -Uri $Url -OutFile $Destination -UseBasicParsing -ErrorAction Stop
    }
    $hash = [Security.Cryptography.SHA256]::Create()
    $stream = [IO.File]::OpenRead($Destination)
    try {
        $actual = ([BitConverter]::ToString($hash.ComputeHash($stream))).Replace('-', '')
    } finally {
        $stream.Dispose()
        $hash.Dispose()
    }
    if ($actual -ine $Sha256) {
        throw "SHA-256 mismatch for $Url (got $actual). Refusing to install."
    }
}

function Install-PortableGo {
    param([Parameter(Mandatory = $true)][string] $ToolsRoot)
    $version = '1.26.10'
    $goHome = Join-Path $ToolsRoot 'go'
    $goExe = Join-Path $goHome 'bin\go.exe'
    if (Test-Path -LiteralPath $goExe -PathType Leaf) {
        $foundVersion = & $goExe version
        if ($LASTEXITCODE -eq 0 -and $foundVersion -match 'go1\.26\.10') {
            Write-Host "Bundled Go $version already installed."
            return $goExe
        }
        throw "Bundled Go is unexpected or corrupted at $goExe. Remove .cusic-tools/go and rerun install.cmd."
    }
    if (Test-Path -LiteralPath $goHome) {
        throw "Incomplete Go installation at $goHome. Remove .cusic-tools/go and rerun install.cmd."
    }

    $temp = Join-Path ([IO.Path]::GetTempPath()) ("cusic-go-" + [guid]::NewGuid().ToString('N'))
    $archive = Join-Path $temp 'go.zip'
    $extract = Join-Path $temp 'unpacked'
    New-Item -ItemType Directory -Path $temp -Force | Out-Null
    try {
        Write-Host "Downloading portable Go $version..."
        Get-VerifiedArchive -Url "https://go.dev/dl/go$version.windows-amd64.zip" -Sha256 '7ecd2d7260e82c59bcbd7f1077a07cee2dea607295b858df67195a1e4e09629b' -Destination $archive
        Expand-Archive -LiteralPath $archive -DestinationPath $extract -ErrorAction Stop
        $source = Join-Path $extract 'go'
        if (-not (Test-Path -LiteralPath (Join-Path $source 'bin\go.exe'))) {
            throw 'Expected go/bin/go.exe in the verified archive.'
        }
        Move-Item -LiteralPath $source -Destination $goHome -ErrorAction Stop
    } finally {
        Remove-Item -LiteralPath $temp -Recurse -Force -ErrorAction SilentlyContinue
    }
    if (-not (Test-Path -LiteralPath $goExe -PathType Leaf)) { throw 'Portable Go install failed' }
    Write-Host "Go $version installed locally."
    return $goExe
}

function Install-PortableUV {
    param([Parameter(Mandatory = $true)][string] $ToolsRoot)
    $version = '0.13.7'
    $uvDir = Join-Path $ToolsRoot 'uv'
    $uvExe = Join-Path $uvDir 'uv.exe'
    if (Test-Path -LiteralPath $uvExe -PathType Leaf) {
        $existingVersion = & $uvExe --version
        if ($LASTEXITCODE -eq 0 -and $existingVersion -match '0\.13\.7') {
            Write-Host 'Bundled uv already installed.'
            return $uvExe
        }
        throw "Bundled uv is unexpected or corrupted at $uvExe. Remove .cusic-tools/uv and rerun install.cmd."
    }
    $temp = Join-Path ([IO.Path]::GetTempPath()) ("cusic-uv-" + [guid]::NewGuid().ToString('N'))
    $archive = Join-Path $temp 'uv.zip'
    $extract = Join-Path $temp 'unpacked'
    New-Item -ItemType Directory -Path $temp -Force | Out-Null
    try {
        Write-Host "Downloading portable uv $version..."
        Get-VerifiedArchive -Url "https://github.com/astral-sh/uv/releases/download/$version/uv-x86_64-pc-windows-msvc.zip" -Sha256 '0cdc5b38fe5a8168ed6a25814f5ba43c21d313dd3187314fccb37b7cec46e021' -Destination $archive
        Expand-Archive -LiteralPath $archive -DestinationPath $extract -ErrorAction Stop
        $matches = @(Get-ChildItem -LiteralPath $extract -File -Recurse -Filter 'uv.exe')
        if ($matches.Count -ne 1) { throw "Expected one uv.exe in verified archive, found $($matches.Count)" }
        New-Item -ItemType Directory -Path $uvDir -Force | Out-Null
        Copy-Item -LiteralPath $matches[0].FullName -Destination $uvExe -ErrorAction Stop
    } finally {
        Remove-Item -LiteralPath $temp -Recurse -Force -ErrorAction SilentlyContinue
    }
    if (-not (Test-Path -LiteralPath $uvExe -PathType Leaf)) { throw 'Portable uv install failed' }
    Write-Host "uv $version installed locally."
    return $uvExe
}

function Install-PythonEnvironment {
    param(
        [Parameter(Mandatory = $true)][string] $RepoRoot,
        [Parameter(Mandatory = $true)][string] $ToolsRoot,
        [Parameter(Mandatory = $true)][string] $UvExe
    )
    $venv = Join-Path $ToolsRoot 'venv'
    $venvPython = Join-Path $venv 'Scripts\python.exe'
    $requirements = Join-Path $RepoRoot 'audio\requirements.txt'

    # The managed Python runtime and pip's wheel/cache downloads are private
    # to cusic, including when a user has another Python installed globally.
    $env:UV_PYTHON_INSTALL_DIR = Join-Path $ToolsRoot 'python'
    $env:UV_CACHE_DIR = Join-Path $ToolsRoot 'cache'
    $env:UV_NO_PROGRESS = '1'
    $env:UV_PYTHON_PREFERENCE = 'only-managed'

    if (-not (Test-Path -LiteralPath $venvPython -PathType Leaf)) {
        Write-Host 'Downloading isolated Python 3.12 runtime with uv...'
        & $UvExe python install 3.12
        if ($LASTEXITCODE -ne 0) { throw "uv python install failed (exit code $LASTEXITCODE)" }
        & $UvExe venv --python 3.12 --python-preference only-managed $venv
        if ($LASTEXITCODE -ne 0) { throw "uv venv failed (exit code $LASTEXITCODE)" }
    }

    Write-Host 'Synchronizing NumPy/SciPy in cusic private virtual environment...'
    & $UvExe pip install --python $venvPython -r $requirements
    if ($LASTEXITCODE -ne 0) { throw "uv pip install failed (exit code $LASTEXITCODE)" }
    & $venvPython -c 'import numpy, scipy; print("Python NumPy/SciPy ready:", numpy.__version__, scipy.__version__)'
    if ($LASTEXITCODE -ne 0) { throw 'NumPy/SciPy verification failed' }
}

try {
    $repoRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
    $binDir = [IO.Path]::GetFullPath((Join-Path $repoRoot 'bin'))
    $exe = Join-Path $binDir 'cusic.exe'

    if ([Environment]::Is64BitOperatingSystem -ne $true -or [Environment]::Is64BitProcess -ne $true) {
        throw 'This installer currently supports Windows x64; Windows ARM64 is not supported yet.'
    }
    if (-not (Test-Path -LiteralPath $binDir -PathType Container)) {
        New-Item -ItemType Directory -Path $binDir -Force | Out-Null
    }
    $toolsRoot = Join-Path $repoRoot '.cusic-tools'
    New-Item -ItemType Directory -Path $toolsRoot -Force | Out-Null

    $goExe = Install-PortableGo -ToolsRoot $toolsRoot
    $env:GOROOT = Join-Path $toolsRoot 'go'
    $env:GOTOOLCHAIN = 'local'

    Push-Location $repoRoot
    try {
        & $goExe build -mod=readonly -o $exe ./cmd/cusic
        if ($LASTEXITCODE -ne 0) { throw "bundled go build failed (exit code $LASTEXITCODE)" }
    } finally {
        Pop-Location
    }
    if (-not (Test-Path -LiteralPath $exe -PathType Leaf)) {
        throw "The build did not produce $exe"
    }

    # Always provision the optional-but-required-for-tempo-render CLI once.
    Install-RubberBand -BinDir $binDir

    $uvExe = Install-PortableUV -ToolsRoot $toolsRoot
    Install-PythonEnvironment -RepoRoot $repoRoot -ToolsRoot $toolsRoot -UvExe $uvExe

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

    Write-Host 'Installed cusic, Go, isolated Python, NumPy/SciPy, and Rubber Band successfully.'
    Write-Host 'Restart your terminal (and VS Code if using its integrated terminal), then run: cusic help'
    exit 0
} catch {
    Write-Error "cusic installation failed: $($_.Exception.Message)"
    exit 1
}
