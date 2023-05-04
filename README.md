fel IgnProt.fi_Offset=19
rätt IgnProt.fi_Offset=2,5
dela värdet på 10


fel Out.X_AccPedal=71
rätt Out.X_AccPedal=15,1
dela värdet på 10

fel Out.fi_Ignition=247
rätt Out.fi_Ignition=26,8
dela värdet på 10

fel Out.PWM_BoostCntrl=20
rätt Out.PWM_BoostCntrl=9,9

fel In.v_Vehicle=1217
rätt In.v_Vehicle=121,7
In.v_Vehicle dela på 10


rätt In.p_AirInlet=-0,485

$env:PKG_CONFIG_PATH="C:\vcpkg\packages\libusb_x86-windows\lib\pkgconfig"; $env:CGO_CFLAGS="-IC:\vcpkg\packages\libusb_x86-windows\include\libusb-1.0"; $env:GOARCH=386; $env:CGO_ENABLED=1; go run -tags combi .

$env:PKG_CONFIG_PATH="C:\vcpkg\packages\libusb_x86-windows\lib\pkgconfig"; $env:CGO_CFLAGS="-IC:\vcpkg\packages\libusb_x86-windows\include\libusb-1.0"; $env:GOARCH=386; $env:CGO_ENABLED=1; fyne package -tags combi --release
