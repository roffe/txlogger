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

if ($release) {
    $cangateway = $true
    $txlogger = $true
    $setup = $true
}

$env:CGO_ENABLED = "1" 
$env:GOGC = "100"
$env:CC = "clang.exe"
$env:CXX = "clang.exe"

$current_path = Get-Location

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
    Write-Host "Building setup.exe"

    $ifpPath = (Get-Location).Path + "\setup.nsi"
    Start-Process -FilePath "C:\Program Files (x86)\NSIS\makensis.exe" -ArgumentList $ifpPath -WorkingDirectory (Get-Location).Path -Wait

    if (-not (Test-Path "setup.exe")) {
        Write-Host "setup.exe not found. Exiting."
        exit
    }

    $winRarPath = "C:\Program Files\WinRAR\WinRAR.exe"
    $filesToAdd = "setup.exe"
    $outputZip = "setup.zip"
    $winRarArgs = "a -m5 -afzip $outputZip $filesToAdd"

    Write-Output "Creating setup.zip"
    Start-Process -FilePath $winRarPath -ArgumentList $winRarArgs -NoNewWindow -Wait
}