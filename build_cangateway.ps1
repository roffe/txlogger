# $invocation = (Get-Variable MyInvocation).Value
# $directorypath = Split-Path $invocation.MyCommand.Path
$env:PKG_CONFIG_PATH = "C:\vcpkg\packages\libusb_x86-windows\lib\pkgconfig"
$env:CGO_CFLAGS = "-IC:\vcpkg\packages\libusb_x86-windows\include\libusb-1.0 -IC:\local\Canlib\INC -IC:\local\CANUSB\include"
$env:CGO_LDFLAGS = "-LC:\local\Canlib\Lib\MS -LC:\local\CANUSB\libs"
$env:GOARCH = "386"
$env:CGO_ENABLED = "1" 
$env:GOGC = "100"
# $env:CC = "C:\\mingw32\\bin\gcc.exe"
# $env:CXX = "C:\\mingw32\\bin\g++.exe"
$env:CC = "clang.exe"
$env:CXX = "clang.exe"
# go-winres simply --icon Icon.png --manifest gui
# go build -tags="canusb,canlib,combi,j2534,kvaser" -ldflags '-s -w -H=windowsgui' ..\gocan\cangateway
go build -tags="j2534" -ldflags '-s -w -H=windowsgui' ..\gocan\cmd\cangateway
