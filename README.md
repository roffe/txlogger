
$env:PKG_CONFIG_PATH="C:\vcpkg\packages\libusb_x86-windows\lib\pkgconfig"; $env:CGO_CFLAGS="-IC:\vcpkg\packages\libusb_x86-windows\include\libusb-1.0"; $env:GOARCH=386; $env:CGO_ENABLED=1; go run -tags combi .

$env:PKG_CONFIG_PATH="C:\vcpkg\packages\libusb_x86-windows\lib\pkgconfig"; $env:CGO_CFLAGS="-IC:\vcpkg\packages\libusb_x86-windows\include\libusb-1.0"; $env:GOARCH=386; $env:CGO_ENABLED=1; fyne package -tags combi --release
