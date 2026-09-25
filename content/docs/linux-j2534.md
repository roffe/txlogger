---
title: J2534 on Linux
weight: 25
---

The vendors of the Drewtech MongoosePro GM II and the Tactrix OpenPort 2.0 only ship
Windows drivers. These two open source drivers make the cables work on Linux:

| Adapter | Driver |
|---|---|
| Drewtech MongoosePro GM II | [libj2534_mongoose](https://github.com/roffe/libj2534_mongoose) |
| Tactrix OpenPort 2.0 | [libj2534_openport2](https://github.com/roffe/libj2534_openport2) |

Each driver is a single C file that builds to one `.so`. Neither driver needs libusb, a udev
rule or root. Both cables show up as a USB serial device (`/dev/ttyACM*`) through the
`cdc_acm` kernel module that every desktop distro already has.

txlogger finds J2534 drivers on Linux the same way other Linux J2534 apps do: it reads every
`~/.passthru/*.json` file. Each file names a driver `.so`. Both drivers install themselves
there.

## 1. Install build tools

You need `git`, `make` and a C compiler.

```sh
# Debian / Ubuntu
sudo apt install git build-essential

# Fedora
sudo dnf install git make gcc

# Arch
sudo pacman -S git base-devel
```

## 2. Get access to the serial port

Your user must be allowed to open `/dev/ttyACM*`. Add yourself to the serial group, then
log out and back in:

```sh
sudo usermod -aG dialout $USER   # Debian, Ubuntu, Fedora
sudo usermod -aG uucp $USER      # Arch
```

Run `groups` to check that the group was added.

## 3. Build and install the driver

### Drewtech MongoosePro GM II

```sh
git clone https://github.com/roffe/libj2534_mongoose
cd libj2534_mongoose
make
make install
```

### Tactrix OpenPort 2.0

```sh
git clone https://github.com/roffe/libj2534_openport2
cd libj2534_openport2
make
make install
```

`make install` copies the driver and its descriptor into your home directory. It does not
need `sudo`:

```
~/.passthru/libj2534_mongoose.so    ~/.passthru/mongoose.json
~/.passthru/libj2534_openport2.so   ~/.passthru/openport2.json
```

You can install both drivers side by side. To update a driver later, run `git pull`,
`make` and `make install` again in its folder.

## 4. Select it in txlogger

Plug in the cable and start txlogger. Open **Settings → CAN Adapter** and pick the J2534 entry:

- `J2534 #0 Drew Technologies MongoosePro GM II`
- `J2534 #1 Tactrix Inc. OpenPort 2.0`

The number depends on how many drivers you have installed.

The Linux AppImage includes J2534 support. If you build txlogger from source, `make` in
the txlogger repo already builds with the `j2534` tag.

## Adapter notes

### MongoosePro GM II

- **The cable needs power from the car.** It will not open without battery voltage on
  OBD pin 16. You get `no voltage on the vehicle connector` otherwise.
- The driver uses the first Drew/Mongoose device under `/dev/serial/by-id/`. To pick a
  specific port, start txlogger with `MONGOOSE_PORT=/dev/ttyACM1 ./txlogger`.
- It does not work with Trionic 5. At T5's 615 kbit/s the cable sends frames but never
  passes received frames back. The firmware causes this, and the driver cannot fix it.

### OpenPort 2.0

- **Remove the microSD card.** With a card inserted the OpenPort can start in standalone
  logging mode, and then it never shows up as a USB device.
- Set `OP2_J2534_LOG` to trace every command and reply. Include this trace when you
  report a bug:

  ```sh
  OP2_J2534_LOG=/tmp/op2.log ./txlogger
  ```

- The OpenPort repo has a small test program that prints the firmware version and
  battery voltage without starting txlogger:

  ```sh
  make test_op2
  ./test_op2
  ```

## Troubleshooting

**No J2534 entry in the adapter list.** Check that `~/.passthru/` has both the `.so` and
the `.json` file. txlogger skips a `.json` whose `.so` is missing.

**Permission denied on `/dev/ttyACM0`.** You are not in the `dialout`/`uucp` group yet, or
you have not logged out and back in since adding it.

**Nothing under `/dev/ttyACM*`.** Run `sudo dmesg | tail` right after plugging in the
cable. You should see a `cdc_acm` line. If you do not, check the cable, the USB port and,
on the OpenPort, the microSD card.
