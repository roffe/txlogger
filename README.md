# txlogger
![Windows Build](https://github.com/roffe/txlogger/actions/workflows/windows-release.yml/badge.svg)

Blazing fast data logging for Trionic 5, 7 & 8 ECU's found in Saab 900, 9000, 9-3 & 9-5 1993-2010
Saab Automobile, Gone but never forgotten

Created after discussions on [TrionicTuning](https://www.trionictuning.com/forum/viewtopic.php?f=34&t=14297)

Built with [goCAN](https://github.com/roffe/gocan)

## Run
    .\build_cangateway.ps1
    .\run.ps1

## Build
    .\build.ps1

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
