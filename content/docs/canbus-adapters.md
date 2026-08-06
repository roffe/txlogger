---
title: Supported CANbus Adapters
weight: 20
---

txlogger talks to the car through [goCAN](https://github.com/roffe/gocan) and supports the
following adapters. Pick the one matching your hardware in **Settings → CAN Adapter**.

## USB

| Adapter | Notes |
|---|---|
| CombiAdapter | Firmware 1.2 or newer recommended |
| Lawicel CANUSB | Via D2XX, canusbdrv.dll or virtual COM port |
| Kvaser (CANlib) | Leaf Light and other Kvaser devices via the Kvaser CANlib driver |
| PEAK PCAN-USB | Via the PEAK driver, Windows only |
| OBDLink SX / EX | Also generic STN1170 / STN2120 boards |
| ELM327 | v1.4 or newer. Beware of cheap clones — many fake ELM327s drop frames |
| CANable (SLCan) | Any SLCan-compatible device |
| Just4Trionic | STM32F103C8T6 based DIY adapter |
| YACA | Yet Another CANbus Adapter |

## J2534 pass-thru (Windows)

Any device with a J2534 driver installed, for example Drewtech Mongoose, Tactrix Openport
and GM MDI. txlogger enumerates installed J2534 drivers automatically.

## WiFi

| Adapter | Notes |
|---|---|
| txbridge | WiFi CANbus bridge by [roffe.nu](https://roffe.nu) |
| OBDX Pro | WiFi mode |

## Other

| Adapter | Notes |
|---|---|
| rCAN | CAN device by [roffe.nu](https://roffe.nu) |
| SocketCAN | Linux only, when building txlogger from source |
| Drewtech Mongoose | Native Linux driver, when building txlogger from source |
