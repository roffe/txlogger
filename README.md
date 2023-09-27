# txlogger
![Windows Build](https://github.com/roffe/txlogger/actions/workflows/go.yml/badge.svg)

Blazing fast data logging for Trionic 7 & 8 ECU's found in Saab 9-5 & 9-3

Created after discussions on [TrionicTuning](https://www.trionictuning.com/forum/viewtopic.php?f=34&t=14297)

Built on top of [goCAN](https://github.com/roffe/gocan) and [Fyne](https://fyne.io/)

“Gone but never forgotten”

## Run
    $env:PKG_CONFIG_PATH="C:\vcpkg\packages\libusb_x86-windows\lib\pkgconfig"; $env:CGO_CFLAGS="-IC:\vcpkg\packages\libusb_x86-windows\include\libusb-1.0"; $env:GOARCH=386; $env:CGO_ENABLED=1; go run -tags combi .

## Build
    $env:PKG_CONFIG_PATH="C:\vcpkg\packages\libusb_x86-windows\lib\pkgconfig"; $env:CGO_CFLAGS="-IC:\vcpkg\packages\libusb_x86-windows\include\libusb-1.0"; $env:GOARCH=386; $env:CGO_ENABLED=1; fyne package -tags combi --release

## Build requirements

libusb from vcpkg for combiadapter support

## Runtime requirements

CombiAdapter support which depends on libusb requires you to install [vc_redist.x86.exe](https://www.microsoft.com/en-gb/download/confirmation.aspx?id=48145)

### Benchmarks

#### EU0D T7 @ 25mhz on bench, 14 symbols

    CANUSB 96 - 102 fps ( com port speed 3000000 and port latency set to 1ms)
    SM2 PRO 109 - 119 fps
    OBDLink SX 87-93 fps (with 1ms latency set)
    CombiAdapter 100 - 106 fps
    Mongoose Pro GM II 101 - 106 fps
    STN2120 97 - 103 fps
    Just4Trionic 19 - 24 fps

## Screenshots

![](txlogger.jpg)
![](txlogger2.jpg)