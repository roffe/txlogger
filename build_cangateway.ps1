$invocation = (Get-Variable MyInvocation).Value
$directorypath = Split-Path $invocation.MyCommand.Path
$env:PKG_CONFIG_PATH = $directorypath + "\vcpkg\packages\libusb_x86-windows\lib\pkgconfig"
$env:CGO_CFLAGS = "-I" + $directorypath + "\vcpkg\packages\libusb_x86-windows\include\libusb-1.0 -IC:\local\Canlib\INC"
$env:CGO_LDFLAGS = "-LC:\local\Canlib\Lib\MS"
$env:GOARCH = "386"
$env:CGO_ENABLED = "1" 
$env:GOGC = "100"
# $env:CC = "C:\\mingw32\\bin\gcc.exe"
# $env:CXX = "C:\\mingw32\\bin\g++.exe"
$env:CC = "clang.exe"
$env:CXX = "clang.exe"
go build -tags="canlib,combi,j2534,kvaser" -ldflags '-s -w -H=windowsgui' ..\gocan\cangateway
