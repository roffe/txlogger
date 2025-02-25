# .\build_cangateway.ps1
$env:PKG_CONFIG_PATH = "C:\vcpkg\packages\libusb_x64-windows\lib\pkgconfig"
$env:CGO_CFLAGS = "-IC:\vcpkg\packages\libusb_x64-windows\include\libusb-1.0 -IC:\local\Canlib\INC"
$env:CGO_LDFLAGS = "-LC:\local\Canlib\Lib\x64"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "1" 
$env:GOGC = "100"
$env:CC = "clang.exe"
$env:CXX = "clang.exe"
Copy-Item -Path $Env:USERPROFILE\Documents\PlatformIO\Projects\txbridge\.pio\build\esp32dev\firmware.bin -Destination .\pkg\ota\
go generate ./...
go run -tags="canusb,combi,canlib,j2534" . $args
