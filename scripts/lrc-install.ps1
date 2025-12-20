# lrc installer for Windows PowerShell
# Usage: iwr -useb https://your-domain/lrc-install.ps1 | iex
#   or:  Invoke-WebRequest -Uri https://your-domain/lrc-install.ps1 -UseBasicParsing | Invoke-Expression

$ErrorActionPreference = "Stop"

# B2 read-only credentials (hardcoded)
$B2_KEY_ID = "00536b4c5851afd0000000006"
$B2_APP_KEY = "K005DV+hNk6/fdQr8oXHmRsdo8U2YAU"
$B2_BUCKET_NAME = "hexmos"
$B2_PREFIX = "lrc"

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

# Authorize with B2
Write-Host -NoNewline "Authorizing with Backblaze B2... "
$authString = "${B2_KEY_ID}:${B2_APP_KEY}"
$authBytes = [System.Text.Encoding]::UTF8.GetBytes($authString)
$authBase64 = [System.Convert]::ToBase64String($authBytes)

try {
    $authResponse = Invoke-RestMethod -Uri "https://api.backblazeb2.com/b2api/v2/b2_authorize_account" `
        -Method Get `
        -Headers @{ "Authorization" = "Basic $authBase64" }
    Write-Host "✓" -ForegroundColor Green
} catch {
    Write-Host "✗" -ForegroundColor Red
    Write-Host "Error: Failed to authorize with B2" -ForegroundColor Red
    Write-Host $_.Exception.Message -ForegroundColor Red
    exit 1
}

$AUTH_TOKEN = $authResponse.authorizationToken
$API_URL = $authResponse.apiUrl
$DOWNLOAD_URL = $authResponse.downloadUrl

# List files in the lrc/ folder to find versions
Write-Host -NoNewline "Finding latest version... "
try {
    $listBody = @{
        bucketId = "33d6ab74ac456875919a0f1d"
        startFileName = "$B2_PREFIX/"
        prefix = "$B2_PREFIX/"
        maxFileCount = 10000
    } | ConvertTo-Json

    $listResponse = Invoke-RestMethod -Uri "$API_URL/b2api/v2/b2_list_file_names" `
        -Method Post `
        -Headers @{ "Authorization" = $AUTH_TOKEN; "Content-Type" = "application/json" } `
        -Body $listBody
} catch {
    Write-Host "✗" -ForegroundColor Red
    Write-Host "Error: Failed to list files from B2" -ForegroundColor Red
    Write-Host $_.Exception.Message -ForegroundColor Red
    exit 1
}

# Extract versions from file names (looking for paths like lrc/vX.Y.Z/)
$versions = $listResponse.files | 
    Where-Object { $_.fileName -match "^$B2_PREFIX/v[0-9]+\.[0-9]+\.[0-9]+/" } | 
    ForEach-Object { 
        if ($_.fileName -match "^$B2_PREFIX/(v[0-9]+\.[0-9]+\.[0-9]+)/") {
            $matches[1]
        }
    } | 
    Select-Object -Unique | 
    Sort-Object -Descending

if ($versions.Count -eq 0) {
    Write-Host "✗" -ForegroundColor Red
    Write-Host "Error: No versions found in $B2_BUCKET_NAME/$B2_PREFIX/" -ForegroundColor Red
    exit 1
}

$LATEST_VERSION = $versions[0]
Write-Host "✓ Latest version: $LATEST_VERSION" -ForegroundColor Green

# Construct download URL
$BINARY_NAME = "lrc.exe"
$DOWNLOAD_PATH = "$B2_PREFIX/$LATEST_VERSION/$PLATFORM/$BINARY_NAME"
$FULL_URL = "$DOWNLOAD_URL/file/$B2_BUCKET_NAME/$DOWNLOAD_PATH"

Write-Host -NoNewline "Downloading lrc $LATEST_VERSION for $PLATFORM... "
$TMP_FILE = [System.IO.Path]::GetTempFileName()
try {
    Invoke-WebRequest -Uri $FULL_URL -OutFile $TMP_FILE -UseBasicParsing -Headers @{ "Authorization" = $AUTH_TOKEN }
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
