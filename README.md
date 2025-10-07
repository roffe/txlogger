# txlogger
![Windows Build](https://github.com/roffe/txlogger/actions/workflows/windows.yml/badge.svg)

![](txlogger.jpg)

Blazing fast data logging for SAAB Trionic ECU's

Trionic 5 - 9000, NG900 & OG9-3
Trionic 7 - OG9-3 & OG9-5
Trionic 8 - NG9-3

SAAB Automobile, Gone but never forgotten

Created after discussions on [TrionicTuning](https://www.trionictuning.com/forum/viewtopic.php?f=34&t=14297)

Built with [goCAN](https://github.com/roffe/gocan)

## Bootstrap the project

Install Golang & C-compiler - [DEVELOPMENT.md](DEVELOPMENT.md)

    .\setup_build_env.ps1
    go get .

## Run
    .\build_cangateway.ps1 # only needs to be done once
    .\run.ps1

## Build
    .\build.ps1

## Build requirements

### libusb *

libusb from vcpkg for combiadapter support

    vcpkg install 'libusb:x64-windows'
    vcpkg install 'libusb:x86-windows'

### CANlib *

https://kvaser.com/single-download/?download_id=47112

Install in the default location `C:\Program Files (x86)\Kvaser\Canlib`

### CANUSB *

https://www.canusb.com/files/canusb_dll_driver.zip
unzip the content of the file into a folder called "canusb" in the root of the project

#### *
Is installed by the setup script

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
