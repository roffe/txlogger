export CC=clang

CANINTERFACES=canlib,canusb,combi,ftdi,j2534,pcan,rcan,socketcan
OTHER=wayland

BUILDTAGS=$(CANINTERFACES),$(OTHER)

ifdef EXTRA_TAGS
BUILDTAGS:=$(CANINTERFACES),$(EXTRA_TAGS)
endif

default: txlogger

pkg/ota/firmware.bin: /home/roffe/Documents/PlatformIO/Projects/txbridge/.pio/build/esp32dev/firmware.bin
	@cp $< $@

txlogger:
	go build -tags=$(BUILDTAGS) -ldflags '-s -w' -o txlogger .

release:
	fyne package -tags=$(BUILDTAGS) --release

debug: clean
	@echo Using compiler "$(CC)"
	-go run -tags=$(BUILDTAGS),debug . 2>&1 | tee run.log

windows:
	CGO_CFLAGS="-Ivcpkg/packages/libusb_x64-windows/include/libusb-1.0" \
	CGO_LDFLAGS="-Lvcpkg/packages/libusb_x64-windows/lib" \
	CGO_ENABLED=1 \
	CC=x86_64-w64-mingw32-gcc \
	GOARCH=amd64 \
	GOOS=windows \
	fyne package --os windows --icon Icon.png -tags=$(BUILDTAGS) --release \
	GOARCH=386 \
	go build -tags=j2534 -ldflags '-s -w' -o j2534proxy.exe ./j2534proxy

windows-dx:
	CGO_CFLAGS="-Ivcpkg/packages/libusb_x64-windows/include/libusb-1.0" \
	CGO_LDFLAGS="-Lvcpkg/packages/libusb_x64-windows/lib" \
	CGO_ENABLED=1 \
	CC=x86_64-w64-mingw32-gcc \
	GOARCH=amd64 \
	GOOS=windows \
	fyne package --os windows --icon Icon.png --name txlogger-dx.exe -tags=$(BUILDTAGS),directx --release \
	GOARCH=386 \
	go build -tags=j2534,dx -ldflags '-s -w' -o j2534proxy.exe ./j2534proxy

run: clean pkg/ota/firmware.bin
	@echo Using compiler "$(CC)"
	-GOEXPERIMENT=simd go run -tags=$(BUILDTAGS) . 2>&1 | tee run.log

clean:
	rm -f cangateway j2534proxy.exe
	rm -f txlogger
.PHONY: j2534proxy
j2534proxy:
	GOOS=windows GOARCH=386 go build -tags="j2534" -ldflags '-s -w' -o j2534proxy.exe ./j2534proxy
