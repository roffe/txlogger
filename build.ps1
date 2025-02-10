.\buildcangw.ps1
#$env:PKG_CONFIG_PATH = "/vcpkg/packages/libusb_x86-windows/lib/pkgconfig"
#$env:CGO_CFLAGS = "-I/vcpkg/packages/libusb_x86-windows/include/libusb-1.0"
#$env:PKG_CONFIG_PATH = "/vcpkg/packages/libusb_x64-windows/lib/pkgconfig"
#$env:CGO_CFLAGS = "-I/vcpkg/packages/libusb_x64-windows/include/libusb-1.0"
#$env:GOARCH = "386"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "1"
$env:GOGC = "100"
$env:CC = "clang.exe"
$env:CXX = "clang.exe"

# Invoke-Expression "rsrc -arch 386 -manifest manifest.xml"
Invoke-Expression "copy $Env:USERPROFILE\Documents\PlatformIO\Projects\txbridge\.pio\build\esp32dev\firmware.bin .\pkg\ota\"
go generate ./...
fyne package --release


