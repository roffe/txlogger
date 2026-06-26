export CC=clang

CANINTERFACES=canlib,canusb,combi,ftdi,j2534,pcan,rcan,socketcan
OTHER=wayland

BUILDTAGS=$(CANINTERFACES),$(OTHER)

ifdef EXTRA_TAGS
BUILDTAGS:=$(CANINTERFACES),$(EXTRA_TAGS)
endif

default: txlogger

cangateway:
	go build -tags="j2534" -ldflags '-s -w' -o cangateway ../gocangateway

txlogger:
	go build -tags=$(BUILDTAGS) -ldflags '-s -w' -o txlogger .

release:
	fyne package -tags=$(BUILDTAGS) --release

debug: clean cangateway
	@echo Using compiler "$(CC)"
	-go run -tags=$(BUILDTAGS),debug . 2>&1 | tee run.log

windows:
	CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc GOARCH=386 GOOS=windows go build -tags="j2534" -ldflags '-s -w' -o cangateway.exe ../gocangateway
	PKG_CONFIG_PATH="./vcpkg/packages/libusb_x64-windows/lib/pkgconfig" CGO_CFLAGS="-I/vcpkg/packages/libusb_x64-windows/include/libusb-1.0" CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc GOARCH=amd64 GOOS=windows go build -tags=$(BUILDTAGS) -ldflags '-s -w' -o txlogger.exe .

run: clean cangateway
	@echo Using compiler "$(CC)"
	-GOEXPERIMENT=simd go run -tags=$(BUILDTAGS) . 2>&1 | tee run.log

clean:
	rm -f cangateway
	rm -f txlogger