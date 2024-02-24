$env:PKG_CONFIG_PATH = "C:\vcpkg\packages\libusb_x86-windows\lib\pkgconfig"
$env:CGO_CFLAGS = "-IC:\vcpkg\packages\libusb_x86-windows\include\libusb-1.0"
$env:GOARCH = "386"
$env:CGO_ENABLED = "1"; 
$env:CC = "C:\\mingw32\\bin\i686-w64-mingw32-gcc.exe"
$env:CXX = "C:\\mingw32\\bin\i686-w64-mingw32-g++.exe"
Invoke-Expression "go generate ./..."
Invoke-Expression "go run -tags combi . $args"