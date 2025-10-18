param(
    [switch]$cangateway,
    [switch]$txlogger,
    [switch]$setup,
    [switch]$release
)

if (-not ($cangateway -or $txlogger -or $setup -or $release)) {
    Write-Host "Please specify at least one of the following switches: -cangateway, -txlogger, -setup, -release"
    exit
}

# Check if WinRAR is installed in common locations
$winrarPaths = @(
    "C:\Program Files\WinRAR\WinRAR.exe",
    "C:\Program Files (x86)\WinRAR\WinRAR.exe"
)

$winrarExe = $null
foreach ($path in $winrarPaths) {
    if (Test-Path $path) {
        $winrarExe = $path
        break
    }
}

New-Item -ItemType Directory -Path "dist" -Force | Out-Null

if ($release) {
    $cangateway = $true
    $txlogger = $true
    $setup = $true
}

$env:CGO_ENABLED = "1" 
$env:GOGC = "100"
$env:CC = "clang.exe"
$env:CXX = "clang++.exe"

$current_path = Get-Location

$env:PATH += ';$current_path\llvm-mingw\bin'

if ($cangateway) {
    Write-Output "Building cangateway.exe"
    $includes = @(
        'C:\Progra~2\Kvaser\Canlib\INC'
    )

    $libs = @(
        'C:\Progra~2\Kvaser\Canlib\Lib\MS'
    )

    $env:CGO_CFLAGS = ($includes | ForEach-Object { '-I' + $_ }) -join ' '
    $env:CGO_LDFLAGS = ($libs | ForEach-Object { '-L' + $_ }) -join ' '
    $env:GOARCH = "386"
    go build -tags="canlib,j2534" -ldflags '-s -w -H=windowsgui' github.com/roffe/gocan/cmd/cangateway
}

if ($txlogger) {
    Write-Output "Building txlogger.exe"
    
    $firmware = "$Env:USERPROFILE\Documents\PlatformIO\Projects\txbridge\.pio\build\esp32dev\firmware.bin"
    if (Test-Path -Path $firmware) {
        Write-Output "Copying firmware.bin to pkg\ota"
        Copy-Item -Path $firmware -Destination ".\pkg\ota\" -Force
    }

    $includes = @(
        "$current_path\vcpkg\packages\libusb_x64-windows\include\libusb-1.0",
        'C:\Progra~2\Kvaser\Canlib\INC',
        "$current_path\canusb\include"
    )

    $libs = @(
        'C:\Progra~2\Kvaser\Canlib\Lib\x64',
        "$current_path\canusb\lib64"
    )

    $env:PKG_CONFIG_PATH = "$current_path\vcpkg\packages\libusb_x64-windows\lib\pkgconfig"
    $env:CGO_CFLAGS = ($includes | ForEach-Object { '-I' + $_ }) -join ' '
    $env:CGO_LDFLAGS = ($libs | ForEach-Object { '-L' + $_ }) -join ' '
    $env:GOARCH = "amd64"
    fyne package -tags="canusb,combi,ftdi,j2534" --release
}

if ($setup) {
    Write-Host "Building txlogger_setup.exe"

    $ifpPath = (Get-Location).Path + "\setup\setup.nsi"
    Start-Process -FilePath "C:\Program Files (x86)\NSIS\makensis.exe" -ArgumentList $ifpPath -WorkingDirectory (Get-Location).Path -NoNewWindow -Wait

    if (-not (Test-Path "txlogger_setup.exe")) {
        Write-Host "txlogger_setup.exe not found. Exiting."
        exit
    }

    $filesToAdd = "txlogger_setup.exe"
    $outputZip = "dist\setup.zip"
    $winRarArgs = "a -m5 -afzip $outputZip $filesToAdd"

    Write-Output "Creating setup.zip"
    Start-Process -FilePath $winRarExe -ArgumentList $winRarArgs -NoNewWindow -Wait
}