$env:PKG_CONFIG_PATH = "/vcpkg/packages/libusb_x86-windows/lib/pkgconfig"
$env:CGO_CFLAGS = "-I/vcpkg/packages/libusb_x86-windows/include/libusb-1.0"
$env:GOARCH = "386"
$env:CGO_ENABLED = "1"

# $env:CC = "C:\\mingw32\\bin\\i686-w64-mingw32-gcc.exe"
# $env:CXX = "C:\\mingw32\\bin\\i686-w64-mingw32-gcc.exe"
Invoke-Expression "rsrc -arch 386 -manifest manifest.xml"
Invoke-Expression "go generate ./..."
Invoke-Expression "fyne package -tags combi --release"
Remove-Item "rsrc_windows_386.syso" -ErrorAction SilentlyContinue

# gtk3_x86-windows
# $env:PKG_CONFIG_PATH = "/vcpkg/packages/libusb_x86-windows/lib/pkgconfig:/vcpkg/packages/gtk3_x86-windows/lib/pkgconfig:/vcpkg/packages/pango_x86-windows/lib/pkgconfig"
# $env:CGO_CFLAGS = "-I/vcpkg/packages/libusb_x86-windows/include/libusb-1.0:/vcpkg/packages/gtk3_x86-windows/include/gtk-3.0:/vcpkg/packages/pango_x86-windows/include/pango-1.0"