# lrc installer for Windows PowerShell
# Usage: iwr -useb https://your-domain/lrc-install.ps1 | iex
#   or:  Invoke-WebRequest -Uri https://your-domain/lrc-install.ps1 -UseBasicParsing | Invoke-Expression

$ErrorActionPreference = "Stop"

# Public release manifest URL
$MANIFEST_URL = "https://f005.backblazeb2.com/file/hexmos/lrc/latest.json"

Write-Host "🚀 lrc Installer" -ForegroundColor Cyan
Write-Host "================" -ForegroundColor Cyan
Write-Host ""

# Detect architecture
$ARCH = $env:PROCESSOR_ARCHITECTURE
switch ($ARCH) {
    "AMD64" { $PLATFORM_ARCH = "amd64" }
    "ARM64" { $PLATFORM_ARCH = "arm64" }
    default {
        Write-Host "Error: Unsupported architecture: $ARCH" -ForegroundColor Red
        exit 1
    }
}

$PLATFORM = "windows-$PLATFORM_ARCH"
Write-Host "✓ Detected platform: $PLATFORM" -ForegroundColor Green

# Prefer Program Files (admin) with fallback to user-local
$isAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
$programFilesTarget = "$env:ProgramFiles\lrc"
$userLocalTarget = "$env:LOCALAPPDATA\Programs\lrc"

# Fetch latest version from public manifest
Write-Host -NoNewline "Fetching latest version from release manifest... "
try {
    $manifest = Invoke-RestMethod -Uri $MANIFEST_URL -Method Get -UseBasicParsing
} catch {
    Write-Host "✗" -ForegroundColor Red
    Write-Host "Error: Failed to fetch release manifest" -ForegroundColor Red
    Write-Host $_.Exception.Message -ForegroundColor Red
    exit 1
}

$LATEST_VERSION = [string]$manifest.latest_version
$DOWNLOAD_BASE = [string]$manifest.download_base
if ([string]::IsNullOrWhiteSpace($LATEST_VERSION) -or [string]::IsNullOrWhiteSpace($DOWNLOAD_BASE)) {
    Write-Host "✗" -ForegroundColor Red
    Write-Host "Error: Release manifest missing latest_version or download_base" -ForegroundColor Red
    exit 1
}

Write-Host "✓ Latest version: $LATEST_VERSION" -ForegroundColor Green

# Construct download URL
$BINARY_NAME = "lrc.exe"
$FULL_URL = "$DOWNLOAD_BASE/$LATEST_VERSION/$PLATFORM/$BINARY_NAME"

Write-Host -NoNewline "Downloading lrc $LATEST_VERSION for $PLATFORM... "
$TMP_FILE = [System.IO.Path]::GetTempFileName()
try {
    Invoke-WebRequest -Uri $FULL_URL -OutFile $TMP_FILE -UseBasicParsing
    Write-Host "✓" -ForegroundColor Green
} catch {
    Write-Host "✗" -ForegroundColor Red
    Write-Host "Error: Failed to download" -ForegroundColor Red
    Write-Host "URL: $FULL_URL" -ForegroundColor Red
    Write-Host $_.Exception.Message -ForegroundColor Red
    Remove-Item $TMP_FILE -ErrorAction SilentlyContinue
    exit 1
}

# Check if file was downloaded
if (-not (Test-Path $TMP_FILE) -or (Get-Item $TMP_FILE).Length -eq 0) {
    Write-Host "✗" -ForegroundColor Red
    Write-Host "Error: Downloaded file is empty or missing" -ForegroundColor Red
    Remove-Item $TMP_FILE -ErrorAction SilentlyContinue
    exit 1
}

# Determine install location (prefer Program Files with admin; fallback to user-local)
$INSTALL_DIR = $programFilesTarget

if (-not $isAdmin) {
    Write-Host -NoNewline "Attempting admin install to $programFilesTarget... "
    try {
        if (-not (Test-Path $programFilesTarget)) {
            New-Item -ItemType Directory -Path $programFilesTarget -Force | Out-Null
        }
        Write-Host "✓" -ForegroundColor Green
    } catch {
        Write-Host "skipped (no admin); using user-local install" -ForegroundColor Yellow
        $INSTALL_DIR = $userLocalTarget
    }
}

if ($INSTALL_DIR -eq $programFilesTarget -and -not $isAdmin) {
    # If we are not admin, we'll try an elevated copy during install step
    if (-not (Test-Path $programFilesTarget)) {
        try { New-Item -ItemType Directory -Path $programFilesTarget -Force | Out-Null } catch { }
    }
}

if ($INSTALL_DIR -eq $userLocalTarget) {
    if (-not (Test-Path $INSTALL_DIR)) { New-Item -ItemType Directory -Path $INSTALL_DIR -Force | Out-Null }
}

$INSTALL_PATH = "$INSTALL_DIR\lrc.exe"

# Install binary
Write-Host -NoNewline "Installing to $INSTALL_PATH... "
try {
    if ($INSTALL_DIR -eq $programFilesTarget -and -not $isAdmin) {
        # Attempt elevated copy
        $copyCommand = "Copy-Item -Path '$TMP_FILE' -Destination '$INSTALL_PATH' -Force"
        $p = Start-Process powershell -ArgumentList "-NoProfile","-Command", $copyCommand -Verb RunAs -Wait -PassThru -ErrorAction Stop
        if ($p.ExitCode -ne 0) { throw "Elevated copy failed with exit code $($p.ExitCode)" }
        Remove-Item $TMP_FILE -ErrorAction SilentlyContinue
        Write-Host "✓" -ForegroundColor Green
    } else {
        Move-Item -Path $TMP_FILE -Destination $INSTALL_PATH -Force
        Write-Host "✓" -ForegroundColor Green
    }
} catch {
    Write-Host "✗" -ForegroundColor Red
    Write-Host "Error: Failed to install to $INSTALL_PATH" -ForegroundColor Red
    Write-Host $_.Exception.Message -ForegroundColor Red
    if ($INSTALL_DIR -eq $programFilesTarget -and -not $isAdmin) {
        Write-Host "Falling back to user-local install..." -ForegroundColor Yellow
        $INSTALL_DIR = $userLocalTarget
        if (-not (Test-Path $INSTALL_DIR)) { New-Item -ItemType Directory -Path $INSTALL_DIR -Force | Out-Null }
        $INSTALL_PATH = "$INSTALL_DIR\lrc.exe"
        try {
            Move-Item -Path $TMP_FILE -Destination $INSTALL_PATH -Force
            Write-Host "Retry install to $INSTALL_PATH... ✓" -ForegroundColor Green
        } catch {
            Write-Host "Retry failed: $_" -ForegroundColor Red
            exit 1
        }
    } else {
        exit 1
    }
}

# Add to PATH if not already there
$currentPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($currentPath -notlike "*$INSTALL_DIR*") {
    Write-Host -NoNewline "Adding $INSTALL_DIR to PATH... "
    try {
        $newPath = "$currentPath;$INSTALL_DIR"
        [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
        # Update current session PATH
        $env:Path = "$env:Path;$INSTALL_DIR"
        Write-Host "✓" -ForegroundColor Green
        Write-Host ""
        Write-Host "Note: You may need to restart your terminal for PATH changes to take effect" -ForegroundColor Yellow
    } catch {
        Write-Host "✗" -ForegroundColor Red
        Write-Host "Warning: Could not add to PATH automatically" -ForegroundColor Yellow
        Write-Host "Please add $INSTALL_DIR to your PATH manually" -ForegroundColor Yellow
    }
}

# Verify installation
Write-Host ""
Write-Host "✓ Installation complete!" -ForegroundColor Green
Write-Host ""

# Try to run lrc version
try {
    & $INSTALL_PATH version
} catch {
    Write-Host "Run '$INSTALL_PATH version' to verify installation" -ForegroundColor Yellow
}

Write-Host ""
Write-Host "Run 'lrc --help' to get started"
Write-Host "(You may need to restart your terminal first)"
