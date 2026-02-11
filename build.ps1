param(
    [switch]$cangateway,
    [switch]$txlogger,
    [switch]$setup,
    [switch]$release,
    [switch]$usegitsrc
)

if (-not ($cangateway -or $txlogger -or $setup -or $release)) {
    Write-Host "Please specify at least one of the following switches: -cangateway, -txlogger, -setup, -release"
    exit
}

New-Item -ItemType Directory -Path "dist" -Force | Out-Null

if ($release) {
    $cangateway = $true
    $txlogger = $true
    $setup = $true
}

$env:CGO_ENABLED = "1" 
$env:GOGC = "100"
$env:CC = "x86_64-w64-mingw32-clang.exe"
$env:CXX = "x86_64-w64-mingw32-clang++.exe"

$current_path = Get-Location

if ($cangateway) {
    Write-Output "Building cangateway.exe"
    #$includes = @(
    #    'C:\Progra~2\Kvaser\Canlib\INC'
    #)

    # $libs = @(
    #     'C:\Progra~2\Kvaser\Canlib\Lib\MS'
    # )
    # $env:PKG_CONFIG_PATH = "$current_path\vcpkg\packages\libusb_x64-windows\lib\pkgconfig"
    # $env:CGO_CFLAGS = ($includes | ForEach-Object { '-I' + $_ }) -join ' '
    # $env:CGO_LDFLAGS = ($libs | ForEach-Object { '-L' + $_ }) -join ' '
    $env:GOARCH = "386"
    if ($usegitsrc) {
        # git clone https://github.com/roffe/gocangateway.git
        # Set-Location -Path ".\gocangateway"
        # go build -tags="canlib,j2534" -ldflags '-s -w -H=windowsgui' -o cangateway.exe .
        # Move-Item -Path ".\cangateway.exe" -Destination "$current_path\cangateway.exe" -Force
        # Set-Location -Path $current_path
        go install -tags="j2534" -ldflags '-s -w -H=windowsgui' github.com/roffe/gocangateway@latest
        Move-Item -Path "$Env:USERPROFILE\go\bin\windows_386\gocangateway.exe" -Destination "$current_path\cangateway.exe" -Force
    }
    else {
        #Set-Location -Path "..\gocangateway"
        go build -tags="j2534" -ldflags '-s -w -H=windowsgui' -o cangateway.exe ..\gocangateway
        #Move-Item -Path ".\cangateway.exe" -Destination "$current_path\cangateway.exe" -Force
        #Set-Location -Path $current_path
    } 
}

if ($txlogger) {
    Write-Output "Building txlogger.exe"
    
    $firmware = "$Env:USERPROFILE\Documents\PlatformIO\Projects\txbridge\.pio\build\esp32dev\firmware.bin"
    if (Test-Path -Path $firmware) {
        Write-Output "Copying firmware.bin to pkg\ota"
        Copy-Item -Path $firmware -Destination ".\pkg\ota\" -Force
    }

    $includes = @(
        "$current_path\vcpkg\packages\libusb_x64-windows\include\libusb-1.0"
        #'C:\Progra~2\Kvaser\Canlib\INC',
        #"$current_path\canusb\include"
    )

    # $libs = @(
    #     'C:\Progra~2\Kvaser\Canlib\Lib\x64',
    #     "$current_path\canusb\lib64"
    # )
    

    $env:PKG_CONFIG_PATH = "$current_path\vcpkg\packages\libusb_x64-windows\lib\pkgconfig"
    $env:CGO_CFLAGS = ($includes | ForEach-Object { '-I' + $_ }) -join ' '
    # $env:CGO_LDFLAGS = ($libs | ForEach-Object { '-L' + $_ }) -join ' '
    $env:GOARCH = "amd64"
    fyne package -tags="canlib,canusb,combi,ftdi,j2534,pcan,rcan" --release
}

if ($setup) {
    Write-Host "Building txlogger_setup.exe"
    $ifpPath = (Get-Location).Path + "\setup\setup.nsi"
    Start-Process -FilePath "C:\Program Files (x86)\NSIS\makensis.exe" -ArgumentList $ifpPath -WorkingDirectory (Get-Location).Path -NoNewWindow -Wait
}