param(
    [switch]$cangateway,
    [switch]$nobuildcangateway
)

$current_path = Get-Location

$env:CGO_ENABLED = "1" 
$env:GOGC = "100"
$env:CC = "clang.exe"
$env:CXX = "clang.exe"

if ($cangateway) {
    $includes = @(
        'C:\Progra~2\Kvaser\Canlib\INC'
    )
    $env:CGO_CFLAGS = ($includes | ForEach-Object { '-I' + $_ }) -join ' '

    $libs = @(
        'C:\Progra~2\Kvaser\Canlib\Lib\MS'
    )
    $env:CGO_LDFLAGS = ($libs | ForEach-Object { '-L' + $_ }) -join ' '

    $env:GOARCH = "386"
    go run -tags="canlib,j2534" github.com/roffe/gocan/cmd/cangateway $args
    exit
}

if (-not $nobuildcangateway) {
    Write-Output "Build cangateway"
    & "$current_path\build.ps1" -cangateway
}

$includes = @(
    "$current_path\vcpkg\packages\libusb_x64-windows\include\libusb-1.0",
    'C:\Progra~2\Kvaser\Canlib\INC',
    "$current_path\canusb\include"
)
$env:CGO_CFLAGS = ($includes | ForEach-Object { '-I' + $_ }) -join ' '

$libs = @(
    'C:\Progra~2\Kvaser\Canlib\Lib\x64',
    "$current_path\canusb\lib64"
)
$env:CGO_LDFLAGS = ($libs | ForEach-Object { '-L' + $_ }) -join ' '

$env:PKG_CONFIG_PATH = "$current_path\vcpkg\packages\libusb_x64-windows\lib\pkgconfig"
$env:GOARCH = "amd64"

if (-not (Test-Path -Path ".\canusbdrv64.dll")) {
    Write-Output "Copy canusbdrv64.dll"
    Copy-Item -Path "canusb\dll64\canusbdrv64.dll" -Destination ".\" -Force
}

if (-not (Test-Path -Path ".\libusb-1.0.dll")) {
    Write-Output "Copy libusb-1.0.dll"
    Copy-Item -Path "vcpkg\packages\libusb_x64-windows\bin\libusb-1.0.dll" -Destination ".\" -Force
}

$firmware = "$Env:USERPROFILE\Documents\PlatformIO\Projects\txbridge\.pio\build\esp32dev\firmware.bin"
if (Test-Path -Path $firmware) {
    Write-Output "Copy firmware.bin to pkg\ota"
    Copy-Item -Path $firmware -Destination ".\pkg\ota\" -Force
}

write-Output "Run txlogger"
go run -tags="canusb,combi,ftdi,j2534" . $args
